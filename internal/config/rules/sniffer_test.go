// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/gitcmd"
)

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// initRepo creates a git repo with one commit containing the given files and
// returns the repo dir and the commit SHA.
func initRepo(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	git("config", "user.email", "t@t.co")
	git("config", "user.name", "t")
	for rel, content := range files {
		writeFile(t, dir, rel, content)
	}
	git("add", "-A")
	git("commit", "-m", "init")
	return dir, git("rev-parse", "HEAD")
}

const (
	objcSource   = "#import \"ViewController.h\"\n\n@implementation ViewController\n@end\n"
	matlabSource = "function y = main(x)\n  y = x + 1;\nend\n"
)

// objcRuleText is the exact text a sniffed .m file must resolve to. Assert on
// identity rather than on wording: objc.md currently ships as a placeholder
// copy of default.md, so any substring check would be testing the placeholder.
func objcRuleText(t *testing.T) string {
	t.Helper()
	r, err := loadObjCRule()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// resolveIn builds a real resolver for dir/ref and resolves path.
func resolveIn(t *testing.T, dir, ref, path string, runner *gitcmd.Runner) string {
	t.Helper()
	setTestHome(t, t.TempDir()) // isolate from a real ~/.opencodereview/rule.json
	r, _, err := NewResolver(dir, "", ResolverOptions{Ref: ref, Runner: runner})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r.Resolve(path)
}

func TestSniffer_WorkingTree(t *testing.T) {
	dir, _ := initRepo(t, map[string]string{
		"ios/ViewController.m": objcSource,
		"Models/main.m":        matlabSource,
		"Models/blank.m":       "\n   \n\t\n",
		"src/model.jl":         objcSource, // ObjC-looking content, non-.m path
	})

	tests := []struct {
		name, path string
		wantObjC   bool
		wantSubstr string // checked when wantObjC is false
	}{
		{name: "objc header resolves to objc rule", path: "ios/ViewController.m", wantObjC: true},
		{name: "matlab function header stays matlab", path: "Models/main.m", wantSubstr: "MATLAB"},
		{name: "blank-only file falls back to matlab", path: "Models/blank.m", wantSubstr: "MATLAB"},
		{name: "missing file falls back to matlab", path: "Models/absent.m", wantSubstr: "MATLAB"},
		{name: "non-.m path never sniffs", path: "src/model.jl", wantSubstr: "Type Stability"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveIn(t, dir, "", tt.path, nil)
			if tt.wantObjC {
				if got != objcRuleText(t) {
					t.Errorf("Resolve(%q): want the objc rule, got %q", tt.path, truncate(got, 80))
				}
				return
			}
			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Resolve(%q): want rule containing %q, got %q", tt.path, tt.wantSubstr, truncate(got, 80))
			}
		})
	}
}

// TestSniffer_ReadsAtRefNotCheckedOut is the reason the sniffer reads through
// `git show <ref>:<path>` rather than the working tree: `ocr review --from/--to`
// can target a ref that is not checked out, and the file may not exist on disk
// at all. Reading the working tree would silently fall back to matlab.md.
func TestSniffer_ReadsAtRefNotCheckedOut(t *testing.T) {
	dir, sha := initRepo(t, map[string]string{"ios/ViewController.m": objcSource})

	// Remove it from the working tree; it now exists only in the commit.
	if err := os.Remove(filepath.Join(dir, "ios/ViewController.m")); err != nil {
		t.Fatal(err)
	}

	t.Run("with ref: still sniffs as objc", func(t *testing.T) {
		got := resolveIn(t, dir, sha, "ios/ViewController.m", nil)
		if got != objcRuleText(t) {
			t.Errorf("expected objc rule read at ref %s, got %q", sha, truncate(got, 80))
		}
	})

	t.Run("with ref via gitcmd.Runner", func(t *testing.T) {
		got := resolveIn(t, dir, sha, "ios/ViewController.m", gitcmd.New(2))
		if got != objcRuleText(t) {
			t.Errorf("expected objc rule via runner, got %q", truncate(got, 80))
		}
	})

	t.Run("without ref: file is gone, falls back to matlab", func(t *testing.T) {
		got := resolveIn(t, dir, "", "ios/ViewController.m", nil)
		if !strings.Contains(got, "MATLAB") {
			t.Errorf("expected matlab fallback with no ref, got %q", truncate(got, 80))
		}
	})

	t.Run("unknown ref degrades to matlab rather than erroring", func(t *testing.T) {
		got := resolveIn(t, dir, "0000000000000000000000000000000000000000", "ios/ViewController.m", nil)
		if !strings.Contains(got, "MATLAB") {
			t.Errorf("expected matlab fallback for a bad ref, got %q", truncate(got, 80))
		}
	})
}

// TestSniffer_UserRuleOutranksSniff guards the layering: the sniffer decorates
// the *system* layer, so a user's own rule for a .m path must still win. If the
// sniffer were wrapped around the whole composed resolver it would short-circuit
// and discard the user rule.
func TestSniffer_UserRuleOutranksSniff(t *testing.T) {
	dir, _ := initRepo(t, map[string]string{"ios/ViewController.m": objcSource})
	setTestHome(t, t.TempDir())

	writeFile(t, dir, ".opencodereview/rule.json",
		`{"rules":[{"path":"**/*.m","rule":"MY PROJECT RULE"}]}`)

	r, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if got := r.Resolve("ios/ViewController.m"); got != "MY PROJECT RULE" {
		t.Errorf("user rule must outrank the objc sniff, got %q", truncate(got, 80))
	}
}

// A user rule with merge_system_rule must receive the sniffed ObjC rule as its
// system half, not the MATLAB rule the path alone would select.
func TestSniffer_MergeSystemRuleUsesSniffedRule(t *testing.T) {
	dir, _ := initRepo(t, map[string]string{"ios/ViewController.m": objcSource})
	setTestHome(t, t.TempDir())

	writeFile(t, dir, ".opencodereview/rule.json",
		`{"rules":[{"path":"**/*.m","rule":"MY PROJECT RULE","merge_system_rule":true}]}`)

	r, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	got := r.Resolve("ios/ViewController.m")
	if !strings.Contains(got, "MY PROJECT RULE") {
		t.Error("merged rule lost the user half")
	}
	if !strings.Contains(got, objcRuleText(t)) {
		t.Errorf("merged rule should carry the sniffed objc rule, got %q", truncate(got, 200))
	}
	if strings.Contains(got, "Indexing, Shapes, and Implicit Expansion") {
		t.Error("merged rule carried the MATLAB rule instead of the sniffed objc rule")
	}
}

// TestSniffer_ResolveDetailKeepsPatternPlain guards the JSON-contract concern:
// Pattern must always stay a plain glob (it flows into delegateRuleGroupJSON's
// "pattern" field alongside a schema_version), so the sniff is recorded in the
// internal-only SniffedAs field instead of an annotated Pattern string.
func TestSniffer_ResolveDetailKeepsPatternPlain(t *testing.T) {
	dir, _ := initRepo(t, map[string]string{
		"ios/ViewController.m": objcSource,
		"Models/main.m":        matlabSource,
	})
	setTestHome(t, t.TempDir())

	r, _, err := NewResolver(dir, "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	dr, ok := r.(DetailResolver)
	if !ok {
		t.Fatal("sniffer-wrapped resolver must still satisfy DetailResolver")
	}

	sniffed := dr.ResolveDetail("ios/ViewController.m")
	if sniffed.Pattern != "**/*.m" {
		t.Errorf("Pattern = %q, want plain glob %q (must not carry a sniff annotation)", sniffed.Pattern, "**/*.m")
	}
	if sniffed.SniffedAs != "objc" {
		t.Errorf("SniffedAs = %q, want %q", sniffed.SniffedAs, "objc")
	}
	if sniffed.Source != "system" {
		t.Errorf("Source = %q, want system", sniffed.Source)
	}

	plain := dr.ResolveDetail("Models/main.m")
	if plain.Pattern != "**/*.m" {
		t.Errorf("unsniffed Pattern = %q, want %q", plain.Pattern, "**/*.m")
	}
	if plain.SniffedAs != "" {
		t.Errorf("unsniffed SniffedAs = %q, want empty", plain.SniffedAs)
	}
}

// The objc rule doc is not referenced from system_rules.json, so it must be
// folded into CanonicalConfig explicitly or editing it would not invalidate the
// run manifest's rule_config_sha256.
func TestSniffer_CanonicalConfigIncludesObjCRule(t *testing.T) {
	setTestHome(t, t.TempDir())
	r, _, err := NewResolver(t.TempDir(), "", ResolverOptions{})
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	cc, ok := r.(interface{ CanonicalConfig() []string })
	if !ok {
		t.Fatal("sniffer-wrapped resolver must still expose CanonicalConfig")
	}
	objcRule, err := loadObjCRule()
	if err != nil {
		t.Fatal(err)
	}
	fields := cc.CanonicalConfig()
	found := false
	for i := 0; i+3 < len(fields); i++ {
		if fields[i+2] == "objc" && fields[i+3] == objcRule {
			found = true
			break
		}
	}
	if !found {
		t.Error("CanonicalConfig must include the objc rule text")
	}
}

func TestLooksLikeObjC(t *testing.T) {
	objc := []string{
		"#import \"a.h\"", "#include <stdio.h>", "#pragma once", "#ifdef DEBUG", "#ifndef FOO_H",
		"@import Foundation;", "@interface Foo", "@implementation Foo", "@class Foo;", "@protocol Foo",
		"//", "/* Copyright 2026. */",
	}
	for _, line := range objc {
		if !looksLikeObjC(line) {
			t.Errorf("looksLikeObjC(%q) = false, want true", line)
		}
	}
	notObjc := []string{"", "function y = f(x)", "classdef Foo", "% a comment", "x = 1;"}
	for _, line := range notObjc {
		if looksLikeObjC(line) {
			t.Errorf("looksLikeObjC(%q) = true, want false", line)
		}
	}
}

// TestSniffer_RealisticObjCHeaders guards the case the sniff used to miss: real
// Objective-C files almost never open with #import — Xcode's file template opens
// with a "//" banner, and most projects put a license header first. Both must
// still sniff as ObjC via the "//"/"/*" signal rather than falling through to
// matlab.md (or, worse, silently landing on it as if it were a real match).
func TestSniffer_RealisticObjCHeaders(t *testing.T) {
	xcodeTemplateHeader := "//\n//  ViewController.m\n//  MyApp\n//\n//  Created by x on 1/1/25.\n//\n\n#import \"ViewController.h\"\n"
	licenseHeaderThenImport := "/* Copyright 2026. */\n#import <Foundation/Foundation.h>\n"

	dir, _ := initRepo(t, map[string]string{
		"MyApp/ViewController.m": xcodeTemplateHeader,
		"Sources/AppDelegate.m":  licenseHeaderThenImport,
	})

	for _, path := range []string{"MyApp/ViewController.m", "Sources/AppDelegate.m"} {
		t.Run(path, func(t *testing.T) {
			got := resolveIn(t, dir, "", path, nil)
			if got != objcRuleText(t) {
				t.Errorf("Resolve(%q): want the objc rule, got %q", path, truncate(got, 80))
			}
		})
	}
}

func TestFirstNonBlankLine(t *testing.T) {
	if got := firstNonBlankLine("\n\n  #import <a.h>\nrest\n"); got != "#import <a.h>" {
		t.Errorf("got %q", got)
	}
	if got := firstNonBlankLine("\n \t\n"); got != "" {
		t.Errorf("blank-only should yield empty, got %q", got)
	}
}
