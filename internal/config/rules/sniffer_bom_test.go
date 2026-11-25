// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func TestSniffer_UTF8BOM(t *testing.T) {
	tests := []struct {
		path, content string
		wantObjC      bool
	}{
		{"import.m", "\ufeff" + objcSource, true},
		{"blank-prefix.m", "\ufeff\r\n \t\r\n" + objcSource, true},
		{"license.m", "\ufeff/* Copyright */\n" + objcSource, true},
		{"indented.m", "\ufeff \t" + objcSource, true},
		{"matlab.m", "\ufeff" + matlabSource, false},
		{"octave.m", "\ufeff# Octave comment\nx = 1;\n", false},
		{"empty.m", "\ufeff\r\n \t\r\n", false},
		{"interior-bom.m", "\n\ufeff" + objcSource, false},
	}
	files := make(map[string]string)
	for _, tt := range tests {
		files[tt.path] = tt.content
	}
	dir, sha := initRepo(t, files)
	setTestHome(t, t.TempDir())
	for _, mode := range []string{"workspace", "ref", "ref-runner"} {
		t.Run(mode, func(t *testing.T) {
			opts := ResolverOptions{}
			if mode != "workspace" {
				opts.Ref = sha
				for _, tt := range tests {
					writeFile(t, dir, tt.path, matlabSource)
				}
			}
			if mode == "ref-runner" {
				opts.Runner = gitcmd.New(2)
			}
			resolver, _, err := NewResolver(dir, "", opts)
			if err != nil {
				t.Fatal(err)
			}
			for _, tt := range tests {
				t.Run(tt.path, func(t *testing.T) {
					got := resolver.Resolve(tt.path)
					if tt.wantObjC && got != objcRuleText(t) {
						t.Error("BOM-prefixed Objective-C source did not receive Objective-C rules")
					}
					if !tt.wantObjC && !strings.Contains(got, "MATLAB") {
						t.Error("non-Objective-C input did not retain the MATLAB fallback")
					}
					detail := resolver.(DetailResolver).ResolveDetail(tt.path)
					if (detail.SniffedAs == "objc") != tt.wantObjC || detail.Rule != got {
						t.Errorf("rule detail disagrees with source classification: %+v", detail)
					}
				})
			}
		})
	}
}

func TestSniffer_UTF8BOMPreservesUserRules(t *testing.T) {
	for _, merge := range []bool{false, true} {
		for _, atRef := range []bool{false, true} {
			t.Run(fmt.Sprintf("merge=%t/ref=%t", merge, atRef), func(t *testing.T) {
				dir, sha := initRepo(t, map[string]string{"source.m": "\ufeff" + objcSource})
				setTestHome(t, t.TempDir())
				writeFile(t, dir, ".opencodereview/rule.json", fmt.Sprintf(
					`{"rules":[{"path":"**/*.m","rule":"PROJECT RULE","merge_system_rule":%t}]}`, merge))
				opts := ResolverOptions{}
				if atRef {
					opts.Ref = sha
				}
				resolver, _, err := NewResolver(dir, "", opts)
				if err != nil {
					t.Fatal(err)
				}
				got := resolver.Resolve("source.m")
				if merge {
					if !strings.Contains(got, "PROJECT RULE") || !strings.Contains(got, objcRuleText(t)) {
						t.Error("merged rule must retain the project rule and the Objective-C system rule")
					}
				} else if got != "PROJECT RULE" {
					t.Error("project rule must keep precedence over content sniffing")
				}
			})
		}
	}
}
