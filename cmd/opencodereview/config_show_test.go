// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func configShowEnv(t *testing.T) string {
	t.Helper()
	home := freshOCRHome(t)
	for _, key := range []string{"OCR_LLM_URL", "OCR_LLM_TOKEN", "OCR_LLM_MODEL", "OCR_LLM_TIMEOUT", "OCR_LLM_EXTRA_HEADERS", "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_MODEL", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(key, "")
	}
	return home
}

func TestConfigShowJSONAndReadOnly(t *testing.T) {
	home := configShowEnv(t)
	for _, args := range [][]string{nil, {"--format", "json"}} {
		cmd := newConfigShowCmd()
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var got llm.EndpointInspection
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Source != "none" || got.CredentialStatus != "not_configured" {
			t.Fatalf("inspection = %+v", got)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".opencodereview")); !os.IsNotExist(err) {
		t.Fatal("show created configuration directory")
	}
}

func TestConfigShowRejectsUnsupportedArguments(t *testing.T) {
	configShowEnv(t)
	for _, args := range [][]string{{"--format", "yaml"}, {"unexpected"}, {"--unknown"}} {
		cmd := newConfigShowCmd()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestRunConfigShowIsOffline(t *testing.T) {
	if os.Getenv("OCR_TEST_CONFIG_SHOW") == "1" {
		os.Args = append([]string{os.Args[0]}, os.Args[3:]...)
		rootCmd.SetArgs(os.Args[1:])
		os.Exit(run())
	}
	home := configShowEnv(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	// A raw listener observes gRPC dialing without needing a collector implementation.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var connections atomic.Int32
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-stopped
	})
	marker := filepath.Join(home, "credential-executed")
	path := filepath.Join(home, ".opencodereview", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"provider": "gateway",
		"custom_providers": map[string]any{"gateway": map[string]any{
			"model": "fixture-model", "protocol": "openai", "url": server.URL + "/secret-endpoint",
			"api_key_cmd":   "echo credential-secret > \"" + filepath.ToSlash(marker) + "\"",
			"extra_headers": map[string]string{"X-Credential": "header-secret"},
		}},
		"telemetry": map[string]any{"enabled": true, "exporter": "otlp", "otlp_endpoint": server.URL},
	}
	before, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, collector := range []struct{ name, endpoint, protocol string }{
		{"config_http", "", "http/protobuf"},
		{"environment_http", server.URL, "http/protobuf"},
		{"environment_grpc", "http://" + listener.Addr().String(), "grpc"},
	} {
		for _, invocation := range []struct {
			name string
			args []string
			json bool
		}{
			{"show", []string{"--color", "never", "config", "show", "--format", "json"}, true},
			{"help_flag", []string{"config", "show", "--help"}, false},
			{"help_command", []string{"help", "config", "show"}, false},
			{"help_prefix_flag", []string{"--color", "never", "help", "config", "show"}, false},
			{"help_suffix_flag", []string{"help", "config", "show", "--color", "never"}, false},
		} {
			t.Run(collector.name+"/"+invocation.name, func(t *testing.T) {
				args := append([]string{"-test.run=^TestRunConfigShowIsOffline$", "--"}, invocation.args...)
				cmd := exec.Command(os.Args[0], args...)
				cmd.Env = append(os.Environ(), "OCR_TEST_CONFIG_SHOW=1", "OCR_ENABLE_TELEMETRY=1", "OTEL_EXPORTER_OTLP_ENDPOINT="+collector.endpoint, "OTEL_EXPORTER_OTLP_PROTOCOL="+collector.protocol, "OTEL_EXPORTER_OTLP_TIMEOUT=1000")
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				output, err := cmd.Output()
				if err != nil || stderr.Len() != 0 {
					t.Fatalf("command failed: %v; stderr: %s", err, stderr.String())
				}
				if invocation.json {
					var got llm.EndpointInspection
					if err := json.Unmarshal(output, &got); err != nil {
						t.Fatalf("output is not standalone JSON: %v; %s", err, output)
					}
					if got.Provider != "gateway" || got.Model != "fixture-model" || got.CredentialStatus != "command_not_executed" {
						t.Fatalf("inspection = %+v", got)
					}
				} else if !strings.Contains(string(output), "ocr config show [flags]") || strings.Contains(string(output), `"Resource"`) {
					t.Fatalf("output is not standalone help: %s", output)
				}
				for _, secret := range []string{"credential-secret", "header-secret", "secret-endpoint"} {
					if strings.Contains(string(output), secret) {
						t.Fatalf("command leaked %s", secret)
					}
				}
			})
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("show made %d HTTP requests", requests.Load())
	}
	if connections.Load() != 0 {
		t.Fatalf("show made %d gRPC connections", connections.Load())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("show executed credential command")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("show modified configuration")
	}
}
