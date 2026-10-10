// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestCheckPresets(t *testing.T) {
	providers := []llm.Provider{
		{
			Name: "alpha", DisplayName: "Alpha", Protocol: llm.ProtocolOpenAIChatCompletions,
			BaseURL: "https://example.com/v1", AuthHeader: "Authorization",
			EnvVar: "ALPHA_API_KEY", Models: []string{"default", "other"},
		},
		{
			Name: "beta", DisplayName: "Beta", Protocol: llm.ProtocolAnthropicBedrock,
			AmbientAuth: true,
		},
	}
	const alpha = `{"name":"alpha","displayName":"Alpha","protocol":"openai","baseUrl":"https://example.com/v1","authHeader":"Authorization","envVar":"ALPHA_API_KEY","models":["default","other"]}`
	const beta = `{"name":"beta","displayName":"Beta","protocol":"anthropic-bedrock","baseUrl":"","envVar":"","ambientAuth":true,"models":[]}`
	valid := "[" + alpha + "," + beta + "]"
	for _, tc := range []struct {
		name, payload, wantErr string
	}{
		{"matching values", valid, ""},
		{"different whitespace", "[\n  " + alpha + ",\n  " + beta + "\n]", ""},
		{"missing provider", "[" + alpha + "]", "provider count"},
		{"extra provider", "[" + alpha + "," + beta + "," + beta + "]", "provider count"},
		{"renamed provider", strings.Replace(valid, `"name":"alpha"`, `"name":"other"`, 1), "provider at index"},
		{"duplicate provider", "[" + alpha + "," + alpha + "]", "provider at index"},
		{"reordered providers", "[" + beta + "," + alpha + "]", "provider at index"},
		{"changed model", strings.Replace(valid, `"other"`, `"changed"`, 1), `models for provider "alpha"`},
		{"reordered models", strings.Replace(valid, `["default","other"]`, `["other","default"]`, 1), `models for provider "alpha"`},
		{"null models", strings.Replace(valid, `"models":[]`, `"models":null`, 1), "must not be null"},
		{"missing models", strings.Replace(valid, `,"models":[]`, "", 1), `missing required field "models"`},
		{"null model entry", strings.Replace(valid, `["default","other"]`, `["default",null]`, 1), "must be a string"},
		{"wrong model type", strings.Replace(valid, `["default","other"]`, `["default",42]`, 1), `decode field "models"`},
		{"models object", strings.Replace(valid, `"models":[]`, `"models":{}`, 1), `decode field "models"`},
		{"changed display name", strings.Replace(valid, `"displayName":"Alpha"`, `"displayName":"Changed"`, 1), "displayName"},
		{"changed protocol", strings.Replace(valid, `"protocol":"openai"`, `"protocol":"anthropic"`, 1), "protocol"},
		{"changed URL", strings.Replace(valid, `"baseUrl":"https://example.com/v1"`, `"baseUrl":"https://other.example/v1"`, 1), "baseUrl"},
		{"changed auth header", strings.Replace(valid, `"authHeader":"Authorization"`, `"authHeader":"x-api-key"`, 1), "authHeader"},
		{"changed environment variable", strings.Replace(valid, `"envVar":"ALPHA_API_KEY"`, `"envVar":"OTHER_API_KEY"`, 1), "envVar"},
		{"changed ambient authentication", strings.Replace(valid, `"ambientAuth":true`, `"ambientAuth":false`, 1), "ambientAuth"},
		{"unknown field", strings.Replace(valid, `"name":"alpha"`, `"name":"alpha","unknown":true`, 1), "unknown field"},
		{"wrong case field", strings.Replace(valid, `"name":"alpha"`, `"Name":"alpha"`, 1), `missing required field "name"`},
		{"wrong case extra field", strings.Replace(valid, `"name":"alpha"`, `"name":"alpha","Name":"alpha"`, 1), "unknown field"},
		{"missing empty URL", strings.Replace(valid, `,"baseUrl":""`, "", 1), `missing required field "baseUrl"`},
		{"null empty URL", strings.Replace(valid, `"baseUrl":""`, `"baseUrl":null`, 1), "must not be null"},
		{"missing empty environment variable", strings.Replace(valid, `,"envVar":""`, "", 1), `missing required field "envVar"`},
		{"null empty environment variable", strings.Replace(valid, `"envVar":""`, `"envVar":null`, 1), "must not be null"},
		{"null optional auth header", strings.Replace(valid, `"name":"beta"`, `"name":"beta","authHeader":null`, 1), "must not be null"},
		{"null optional ambient authentication", strings.Replace(valid, `"name":"alpha"`, `"name":"alpha","ambientAuth":null`, 1), "must not be null"},
		{"wrong scalar type", strings.Replace(valid, `"displayName":"Alpha"`, `"displayName":42`, 1), `decode field "displayName"`},
		{"null provider", "[" + alpha + ",null]", `missing required field "name"`},
		{"invalid JSON", "[", "decode provider presets"},
		{"null catalog", "null", "must be an array"},
		{"trailing JSON", valid + " []", "exactly one JSON array"},
		{"trailing invalid content", valid + " invalid", "exactly one JSON array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("// Different header formatting is allowed.\nimport type { OcrProviderPreset } from './providers';\n" + presetDeclaration + "\n" + tc.payload + ";\n")
			err := checkPresets(data, providers)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want a diagnostic containing %q", err, tc.wantErr)
			}
		})
	}
	if err := checkPresets([]byte("import type { OcrProviderPreset } from './providers';\n"+presetDeclaration+" [];\n"), nil); err != nil {
		t.Fatalf("an empty array must match an empty registry: %v", err)
	}
	if err := checkPresets([]byte("export const OTHER = [];\n"), nil); err == nil || !strings.Contains(err.Error(), "missing PROVIDER_PRESETS declaration") {
		t.Fatalf("missing declaration error = %v", err)
	}
}

func TestCheckPresetsImport(t *testing.T) {
	const validImport = "import type { OcrProviderPreset } from './providers';"
	for _, tc := range []struct {
		name, preamble string
		valid          bool
	}{
		{"matching", validImport, true},
		{"double quotes and whitespace", "\nimport type {\n OcrProviderPreset\n} from \"./providers\";\n", true},
		{"no semicolon", "import type { OcrProviderPreset } from './providers'", true},
		{"no semicolon with comment", "import type { OcrProviderPreset } from './providers' // Type import", true},
		{"trailing block comment", "import type { OcrProviderPreset } from './providers' /* Type import */", true},
		{"block comment before semicolon", "import type { OcrProviderPreset } from './providers' /* Type import */;", true},
		{"header comments", "// License\n/* Generated file */\n" + validImport + " // Type import\n", true},
		{"missing import", "", false},
		{"wrong module", "import type { OcrProviderPreset } from './missing-providers';", false},
		{"wrong binding", "import type { OtherPreset } from './providers';", false},
		{"commented import", "// " + validImport + "\n", false},
		{"block-commented import", "/* " + validImport + " */", false},
		{"commented correct import before wrong module", "// " + validImport + "\nimport type { OcrProviderPreset } from './missing-providers';", false},
		{"wrong import between comments", "/* Header */\nimport type { OcrProviderPreset } from './missing-providers';\n/* Header */\n" + validImport, false},
		{"block comments do not nest", "/* Outer /* Inner */\nimport type { OcrProviderPreset } from './missing-providers';\n// */\n" + validImport, false},
		{"duplicate import", validImport + "\n" + validImport, false},
		{"unterminated comment", "/* Header\n" + validImport, false},
		{"unterminated trailing comment", strings.TrimSuffix(validImport, ";") + " /* Type import", false},
		{"overlapping comment delimiters", "/*/\n" + validImport, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.preamble + "\n" + presetDeclaration + " [];\n")
			err := checkPresets(data, nil)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "OcrProviderPreset import") || !strings.Contains(err.Error(), "./providers") {
				t.Fatalf("expected an import diagnostic, got %v", err)
			}
		})
	}
	if err := checkPresets([]byte(validImport+"\n// "+presetDeclaration+" [];\n"), nil); err == nil || !strings.Contains(err.Error(), "OcrProviderPreset import") {
		t.Fatalf("a commented catalog declaration must be rejected, got %v", err)
	}
}

func TestCheckPresetsImportWhitespace(t *testing.T) {
	const validImport = "import type { OcrProviderPreset } from './providers';"
	for _, tc := range []struct {
		name, space string
		valid       bool
	}{
		{"tab", "\t", true},
		{"line feed", "\n", true},
		{"vertical tab", "\v", true},
		{"form feed", "\f", true},
		{"carriage return", "\r", true},
		{"nonbreaking space", "\u00a0", true},
		{"thin space", "\u2009", true},
		{"ideographic space", "\u3000", true},
		{"byte order mark", "\ufeff", true},
		{"line separator", "\u2028", true},
		{"paragraph separator", "\u2029", true},
		{"next line is not whitespace", "\u0085", false},
		{"zero width space is not whitespace", "\u200b", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(tc.space + strings.ReplaceAll(validImport, " ", tc.space) + tc.space + "\n" + presetDeclaration + " [];\n")
			if err := checkPresets(data, nil); (err == nil) != tc.valid {
				t.Fatalf("valid = %t, error = %v", tc.valid, err)
			}
		})
	}
}

func TestCheckPresetsImportLineTerminators(t *testing.T) {
	const validImport = "import type { OcrProviderPreset } from './providers';"
	const wrongImport = "import type { OcrProviderPreset } from './missing-providers';"
	for _, line := range []struct{ name, separator string }{
		{"LF", "\n"}, {"CRLF", "\r\n"}, {"CR", "\r"}, {"LS", "\u2028"}, {"PS", "\u2029"},
	} {
		t.Run(line.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, preamble string
				valid          bool
			}{
				{"header comment", "// Header" + line.separator + validImport, true},
				{"no semicolon with comment", strings.TrimSuffix(validImport, ";") + " // Type import" + line.separator, true},
				{"wrong import after comment", validImport + "\n// Header" + line.separator + wrongImport, false},
				{"wrong import after trailing comment", strings.TrimSuffix(validImport, ";") + " // Type import" + line.separator + wrongImport, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					data := []byte(tc.preamble + "\n" + presetDeclaration + " [];\n")
					err := checkPresets(data, nil)
					if (err == nil) != tc.valid {
						t.Fatalf("valid = %t, error = %v", tc.valid, err)
					}
				})
			}
		})
	}
}

func TestCheckRequiresExistingFile(t *testing.T) {
	for _, paths := range [][2]string{{"", "providers.kt"}, {"providers.ts", ""}} {
		if err := check(paths[0], paths[1]); err == nil || !strings.Contains(err.Error(), "-output") || !strings.Contains(err.Error(), "-kotlin-output") {
			t.Fatalf("missing output path error = %v", err)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.ts")
	kotlinPath := filepath.Join(dir, "missing.kt")
	if err := check(path, kotlinPath); err == nil || !strings.Contains(err.Error(), "read provider presets") {
		t.Fatalf("missing artifact error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("check must not create a missing artifact: %v", err)
	}
	data, err := render(llm.ListProviders())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := check(path, kotlinPath); err == nil || !strings.Contains(err.Error(), "read Kotlin provider names") {
		t.Fatalf("missing Kotlin artifact error = %v", err)
	}
	if _, err := os.Stat(kotlinPath); !os.IsNotExist(err) {
		t.Fatalf("check must not create a missing Kotlin artifact: %v", err)
	}
}

func TestCheckRejectsDuplicateRegistryModels(t *testing.T) {
	providers := []llm.Provider{{Name: "example", Models: []string{"duplicate", "duplicate"}}}
	if err := checkPresets(nil, providers); err == nil || !strings.Contains(err.Error(), `provider "example" has duplicate model "duplicate"`) {
		t.Fatalf("invalid source registry must be rejected before comparison: %v", err)
	}
}

func TestCheckDoesNotRewriteArtifact(t *testing.T) {
	valid, err := render(llm.ListProviders())
	if err != nil {
		t.Fatal(err)
	}
	validKotlin := renderKotlin(llm.ListProviders())
	for _, tc := range []struct {
		name         string
		data, kotlin []byte
		inconsistent bool
	}{
		{"matching", valid, validKotlin, false},
		{"inconsistent frontend", []byte("import type { OcrProviderPreset } from './providers';\n" + presetDeclaration + " [];\n"), validKotlin, true},
		{"inconsistent Kotlin", valid, []byte("package com.alibaba.opencodereview.idea.services\n" + kotlinDeclaration + ")\n"), true},
		{"wrong import path", bytes.Replace(valid, []byte("from './providers'"), []byte("from './missing-providers'"), 1), validKotlin, true},
		{"missing import", bytes.Replace(valid, []byte("import type { OcrProviderPreset } from './providers';"), nil, 1), validKotlin, true},
		{"wrong Kotlin package", valid, bytes.Replace(validKotlin, []byte("package com.alibaba.opencodereview.idea.services"), []byte("package incorrect.services"), 1), true},
		{"missing Kotlin package", valid, bytes.Replace(validKotlin, []byte("package com.alibaba.opencodereview.idea.services"), nil, 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "providers.ts")
			kotlinPath := filepath.Join(dir, "providers.kt")
			modified := time.Unix(1600000000, 0)
			artifacts := map[string][]byte{path: tc.data, kotlinPath: tc.kotlin}
			for artifact, data := range artifacts {
				if err := os.WriteFile(artifact, data, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(artifact, modified, modified); err != nil {
					t.Fatal(err)
				}
			}
			err := check(path, kotlinPath)
			if !tc.inconsistent && err != nil {
				t.Fatal(err)
			}
			if tc.inconsistent && (err == nil || !strings.Contains(err.Error(), "go generate ./internal/llm")) {
				t.Fatalf("mismatch must include the regeneration command: %v", err)
			}
			for artifact, data := range artifacts {
				got, err := os.ReadFile(artifact)
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(artifact)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, data) || !info.ModTime().Equal(modified) {
					t.Fatalf("check must not rewrite %s, even when it is inconsistent", artifact)
				}
			}
		})
	}
}
