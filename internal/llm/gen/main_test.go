// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestGeneratedProvidersUpToDate(t *testing.T) {
	want, err := render(llm.ListProviders())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "..", "extensions", "vscode", "src", "shared", "providers.generated.ts")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("VS Code provider presets are stale; run go generate ./internal/llm from the repository root")
	}
}

func TestRenderPreservesMetadataAndModelOrder(t *testing.T) {
	providers := []llm.Provider{
		{
			Name: "example", DisplayName: "Quoted \"name\" <with> & characters",
			Protocol: llm.ProtocolAnthropic, BaseURL: "https://example.com/v1",
			AuthHeader: "x-api-key", EnvVar: "EXAMPLE_API_KEY",
			Models: []string{"z-default", "a-model", "quote\"slash\\newline\n", "z-default"},
		},
		{
			Name: "ambient", DisplayName: "Ambient credentials",
			Protocol: llm.ProtocolAnthropicBedrock, AmbientAuth: true,
		},
		{
			Name: "responses", Protocol: llm.ProtocolOpenAIResponses,
			Models: []string{"responses-model"},
		},
	}
	data, err := render(providers)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("\r")) {
		t.Fatal("generated output must use LF line endings")
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSuffix(bytes.TrimPrefix(data, []byte(header)), []byte(";\n")), &got); err != nil {
		t.Fatalf("generated payload must be valid JSON: %v", err)
	}
	want := []map[string]any{
		{
			"name": "example", "displayName": "Quoted \"name\" <with> & characters",
			"protocol": "anthropic", "baseUrl": "https://example.com/v1",
			"authHeader": "x-api-key", "envVar": "EXAMPLE_API_KEY",
			"models": []any{"z-default", "a-model", "quote\"slash\\newline\n", "z-default"},
		},
		{
			"name": "ambient", "displayName": "Ambient credentials",
			"protocol": "anthropic-bedrock", "baseUrl": "", "envVar": "",
			"ambientAuth": true, "models": []any{},
		},
		{
			"name": "responses", "displayName": "", "protocol": "openai-responses",
			"baseUrl": "", "envVar": "", "models": []any{"responses-model"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generated metadata = %#v, want %#v", got, want)
	}
}

func TestGenerateRequiresOutput(t *testing.T) {
	err := generate("")
	if err == nil {
		t.Fatal("expected an error for a missing output path")
	}
	if !strings.Contains(err.Error(), "-output") || !strings.Contains(err.Error(), "go generate ./internal/llm") {
		t.Fatalf("expected the required flag and regeneration command in the error, got %v", err)
	}
}

func TestGenerate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.ts")
	if err := generate(path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := generate(path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("generating twice must produce identical bytes")
	}
	if err := generate(t.TempDir()); err == nil || !strings.Contains(err.Error(), "write provider presets") {
		t.Fatalf("expected a contextual write error, got %v", err)
	}
}
