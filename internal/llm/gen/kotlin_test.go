// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestQuoteKotlinString(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"alpha-beta", `"alpha-beta"`},
		{"quote\"slash\\", `"quote\"slash\\"`},
		{"$name ${value}", `"\u0024name \u0024{value}"`},
		{"\x00\b\t\n\f\r", `"\u0000\u0008\u0009\u000a\u000c\u000d"`},
		{"\u00e9\U0001f600", `"\u00e9\ud83d\ude00"`},
	} {
		got := quoteKotlinString(tc.input)
		if got != tc.want {
			t.Errorf("quoteKotlinString(%q) = %s, want %s", tc.input, got, tc.want)
		}
		var decoded string
		if err := json.Unmarshal([]byte(got), &decoded); err != nil || decoded != tc.input {
			t.Errorf("quoted Kotlin name must retain its value as JSON: %q, %v", decoded, err)
		}
	}
}

func TestRenderKotlin(t *testing.T) {
	providers := []llm.Provider{{Name: "alpha"}, {Name: "beta"}}
	data := renderKotlin(providers)
	if !bytes.Contains(data, []byte("package com.alibaba.opencodereview.idea.services\n")) {
		t.Fatal("generated Kotlin must be in the host services package")
	}
	if !bytes.HasSuffix(data, []byte("    \"alpha\",\n    \"beta\",\n)\n")) {
		t.Fatalf("generated Kotlin did not retain provider order: %s", data)
	}
	if bytes.Contains(data, []byte("\r")) {
		t.Fatal("generated Kotlin must use LF line endings")
	}
	if err := checkKotlinNames(renderKotlin(nil), nil); err != nil {
		t.Fatalf("empty registry must produce a valid empty Kotlin set: %v", err)
	}
}

func TestCheckKotlinNames(t *testing.T) {
	providers := []llm.Provider{{Name: "alpha"}, {Name: "beta"}}
	for _, tc := range []struct{ name, payload, wantErr string }{
		{"matching", `"alpha", "beta",)`, ""},
		{"no trailing comma", `"alpha", "beta")`, ""},
		{"missing provider", `"alpha",)`, "provider count"},
		{"extra provider", `"alpha", "beta", "gamma",)`, "provider count"},
		{"reordered providers", `"beta", "alpha",)`, "provider at index"},
		{"duplicate provider", `"alpha", "alpha",)`, "provider at index"},
		{"renamed provider", `"alpha", "changed",)`, "provider at index"},
		{"null name", `"alpha", null,)`, "must be a string"},
		{"invalid literal", `"alpha", 42,)`, "decode Kotlin provider names"},
		{"unterminated set", `"alpha", "beta",`, "closing parenthesis"},
		{"trailing code", `"alpha", "beta",); other()`, "decode Kotlin provider names"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("package com.alibaba.opencodereview.idea.services\n" + kotlinDeclaration + "\n" + tc.payload + "\n")
			err := checkKotlinNames(data, providers)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want a diagnostic containing %q", err, tc.wantErr)
			}
		})
	}
	if err := checkKotlinNames([]byte("setOf()"), nil); err == nil || !strings.Contains(err.Error(), "missing generatedPresetProviderNames") {
		t.Fatalf("missing declaration error = %v", err)
	}
}

func TestCheckKotlinPackage(t *testing.T) {
	const validPackage = "package com.alibaba.opencodereview.idea.services"
	for _, tc := range []struct {
		name, preamble string
		valid          bool
	}{
		{"matching", validPackage, true},
		{"whitespace and semicolon", "\npackage\tcom.alibaba.opencodereview.idea.services;\n", true},
		{"form feed whitespace", "\fpackage\fcom.alibaba.opencodereview.idea.services\f;\n", true},
		{"trailing comment", validPackage + " // Host package", true},
		{"trailing block comment", validPackage + " /* Host package */", true},
		{"nested trailing block comment", validPackage + " /* Outer /* Inner */ End */", true},
		{"block comment before semicolon", validPackage + " /* Host package */;", true},
		{"header comments", "// License\n/* Generated file */\n" + validPackage + "\n// Provider names\n", true},
		{"nested header comments", "/* Outer /* Inner */ End */\n" + validPackage, true},
		{"CR header comment", "// Header\r" + validPackage, true},
		{"CR trailing comment", validPackage + " // Host package\r", true},
		{"missing package", "", false},
		{"wrong package", "package incorrect.services", false},
		{"package suffix", validPackage + ".incorrect", false},
		{"commented package", "// " + validPackage + "\n", false},
		{"block-commented package", "/* " + validPackage + " */", false},
		{"package inside nested comment", "/* Outer /* Inner */\n" + validPackage + "\n// */", false},
		{"commented correct package before wrong package", "// " + validPackage + "\npackage incorrect.services", false},
		{"wrong package after CR comment", validPackage + "\n// Header\rpackage incorrect.services", false},
		{"wrong package after CR trailing comment", validPackage + " // Host package\rpackage incorrect.services", false},
		{"wrong package between comments", "/* Header */\npackage incorrect.services\n/* Header */\n" + validPackage, false},
		{"duplicate package", validPackage + "\n" + validPackage, false},
		{"unterminated comment", "/* Header\n" + validPackage, false},
		{"unterminated trailing comment", validPackage + " /* Host package", false},
		{"overlapping comment delimiters", "/*/\n" + validPackage, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.preamble + "\n" + kotlinDeclaration + ")\n")
			err := checkKotlinNames(data, nil)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "Kotlin package") || !strings.Contains(err.Error(), "com.alibaba.opencodereview.idea.services") {
				t.Fatalf("expected a package diagnostic, got %v", err)
			}
		})
	}
	if err := checkKotlinNames([]byte(validPackage+"\n// "+kotlinDeclaration+")\n"), nil); err == nil || !strings.Contains(err.Error(), "Kotlin package") {
		t.Fatalf("a commented provider declaration must be rejected, got %v", err)
	}
}
