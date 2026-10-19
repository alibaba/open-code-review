// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	ocrmcp "github.com/alibaba/open-code-review/internal/mcp"
	"github.com/alibaba/open-code-review/internal/plugin"
	"github.com/alibaba/open-code-review/internal/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestScanPluginSetupErrorsRecordTrace(t *testing.T) {
	for _, mode := range []string{"canceled", "collision"} {
		t.Run(mode, func(t *testing.T) {
			store := pluginRuntimeHome(t)
			marker := filepath.Join(t.TempDir(), "started")
			installRuntimePlugin(t, store, marker, true)
			repo := initRulesCheckTestRepo(t)
			gitCommitFile(t, repo, "main.go", "package main\n", "initial")
			t.Setenv("OCR_LLM_URL", "http://127.0.0.1:1/v1/messages")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "test")
			t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
			if mode == "collision" {
				path, err := defaultConfigPath()
				if err != nil {
					t.Fatal(err)
				}
				cfg := &Config{MCPServers: map[string]MCPServerConfig{"plugin/runtime/tools": {Command: "user-command"}}}
				if err := saveConfig(path, cfg); err != nil {
					t.Fatal(err)
				}
			}
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
			ctx, span := provider.Tracer("test").Start(context.Background(), "scan-caller")
			defer span.End()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			err := executeScanContext(ctx, scanOptions{repoDir: repo, paths: "main.go", outputFormat: "json", audience: "agent", concurrency: 1, maxGitProcs: 2})
			if err == nil || (mode == "canceled" && !errors.Is(err, context.Canceled)) || (mode == "collision" && !strings.Contains(err.Error(), "conflicts")) {
				t.Fatalf("unexpected setup error: %v", err)
			}
			span.End()
			ended := recorder.Ended()
			if len(ended) != 1 || ended[0].Status().Code != codes.Error || ended[0].Status().Description != err.Error() {
				t.Fatalf("setup failure missing from trace: %+v", ended)
			}
			events := ended[0].Events()
			if len(events) != 1 || events[0].Name != "exception" {
				t.Fatalf("setup error event missing: %+v", events)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("failed setup started a plugin: %v", err)
			}
		})
	}
}

const pluginRuntimeServerEnv = "_OCR_PLUGIN_RUNTIME_SERVER"

// TestPluginMCPServerProcess re-executes the test binary as a real stdio server.
func TestPluginMCPServerProcess(t *testing.T) {
	if os.Getenv(pluginRuntimeServerEnv) != "1" {
		return
	}
	if marker := os.Getenv("_OCR_PLUGIN_RUNTIME_MARKER"); marker != "" {
		if err := os.WriteFile(marker, []byte("started"), 0600); err != nil {
			os.Exit(2)
		}
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "plugin-runtime-test", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "plugin_probe", Description: "Read plugin runtime context", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			data, _ := json.Marshal(map[string]string{
				"cwd": cwd, "env": os.Getenv("_OCR_PLUGIN_RUNTIME_DIR"), "arg": os.Args[len(os.Args)-1],
				"literal": os.Getenv("_OCR_PLUGIN_RUNTIME_LITERAL"),
			})
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
		})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func pluginRuntimeHome(t *testing.T) *plugin.Store {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	store, err := plugin.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func installRuntimePlugin(t *testing.T, store *plugin.Store, marker string, enabled bool) plugin.Installed {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manifest := plugin.Manifest{
		SchemaVersion: 1, ID: "runtime", Name: "Runtime test", Version: "1.0", Rules: []string{"rule.json"},
		MCPServers: map[string]plugin.MCPServer{"tools": {
			Command: executable, Args: []string{"-test.run=^TestPluginMCPServerProcess$", "${plugin_dir}"},
			Env:   []string{pluginRuntimeServerEnv + "=1", "GORACE=atexit_sleep_ms=0", "_OCR_PLUGIN_RUNTIME_MARKER=" + marker, "_OCR_PLUGIN_RUNTIME_DIR=${plugin_dir}", "_OCR_PLUGIN_RUNTIME_LITERAL=$OCR_NO_EXPAND"},
			Tools: []string{"plugin_probe"},
		}},
	}
	dir := t.TempDir()
	data, _ := json.Marshal(manifest)
	for file, content := range map[string][]byte{
		plugin.ManifestFilename: data,
		"rule.json":             []byte(`{"rules":[{"path":"**/*.go","rule":"Runtime plugin rule marker"}]}`),
	} {
		if err := os.WriteFile(filepath.Join(dir, file), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	installed, err := store.Install(context.Background(), dir, enabled)
	if err != nil {
		t.Fatal(err)
	}
	return installed
}

func TestPluginMCPConfigPreservesUserConfig(t *testing.T) {
	user := &Config{MCPServers: map[string]MCPServerConfig{"tools": {Command: "user-command", Env: []string{"SECRET=value"}}}}
	before, _ := json.Marshal(user)
	installed := []plugin.Installed{{Enabled: true, Dir: "/installed package", Manifest: plugin.Manifest{ID: "extra", MCPServers: map[string]plugin.MCPServer{
		"tools":  {Command: "${plugin_dir}/tool", Args: []string{"${plugin_dir}", "$UNEXPANDED"}, Env: []string{"ROOT=${plugin_dir}"}, Tools: []string{"probe"}},
		"remote": {Type: "remote", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "Bearer $TOKEN"}},
	}}}}
	cfg, err := pluginMCPConfig(user, installed)
	if err != nil {
		t.Fatal(err)
	}
	stdio := cfg.MCPServers["plugin/extra/tools"]
	if stdio.Command != "/installed package/tool" || !reflect.DeepEqual(stdio.Args, []string{"/installed package", "$UNEXPANDED"}) || stdio.Env[0] != "ROOT=/installed package" || stdio.Setup != "" {
		t.Fatalf("converted server = %+v", stdio)
	}
	cfg.MCPServers["plugin/extra/remote"].Headers["Authorization"] = "changed"
	stdio.Args[0] = "changed"
	stdio.Tools[0] = "changed"
	if installed[0].Manifest.MCPServers["remote"].Headers["Authorization"] != "Bearer $TOKEN" || installed[0].Manifest.MCPServers["tools"].Args[0] != "${plugin_dir}" || installed[0].Manifest.MCPServers["tools"].Tools[0] != "probe" {
		t.Fatal("runtime mutated the plugin manifest")
	}
	after, _ := json.Marshal(user)
	if !bytes.Equal(before, after) {
		t.Fatal("runtime mutated user MCP config")
	}
	user.MCPServers["plugin/extra/tools"] = MCPServerConfig{Command: "collision"}
	if _, err := pluginMCPConfig(user, installed); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("collision error = %v", err)
	}
	if _, err := pluginMCPConfig(nil, append(installed, installed...)); err == nil {
		t.Fatal("duplicate plugin server was accepted")
	}
	installed[0].Enabled = false
	if cfg, err := pluginMCPConfig(user, installed); err != nil || cfg != nil {
		t.Fatalf("disabled plugin config = %+v, %v", cfg, err)
	}
}

func TestPluginMCPPlatformPlaceholders(t *testing.T) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	p := plugin.Installed{Enabled: true, Dir: "/package ${os} & spaces", Manifest: plugin.Manifest{ID: "platform", MCPServers: map[string]plugin.MCPServer{
		"tool": {Command: "${plugin_dir}/${os}-${arch}/tool${exe}", Args: []string{"${os}", "${arch}", "${exe}", "${plugin_dir}", "$UNCHANGED"}, Env: []string{"PLATFORM=${os}-${arch}", "ROOT=${plugin_dir}"}},
	}}}
	cfg, err := pluginMCPConfig(nil, []plugin.Installed{p})
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.MCPServers["plugin/platform/tool"]
	if got.Command != p.Dir+"/"+runtime.GOOS+"-"+runtime.GOARCH+"/tool"+ext || !reflect.DeepEqual(got.Args, []string{runtime.GOOS, runtime.GOARCH, ext, p.Dir, "$UNCHANGED"}) || !reflect.DeepEqual(got.Env, []string{"PLATFORM=" + runtime.GOOS + "-" + runtime.GOARCH, "ROOT=" + p.Dir}) {
		t.Fatalf("platform expansion = %+v", got)
	}
}

func TestInstalledPluginMCPRuntimeAndDisable(t *testing.T) {
	store := pluginRuntimeHome(t)
	marker := filepath.Join(t.TempDir(), "started")
	installed := installRuntimePlugin(t, store, marker, true)
	repo := t.TempDir()
	r, _, err := rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// A state change after loading rules must not change this run's snapshot.
	if err := store.SetEnabled("runtime", false); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	clients, err := initRuntimeMCPClients(context.Background(), nil, r, reg, repo, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer closeReviewMCPClients(clients)
	if len(clients) != 1 || clients[0].Name() != "plugin/runtime/tools" {
		t.Fatalf("plugin clients = %v", clients)
	}
	defs := ocrmcp.CollectToolDefs(clients, reg)
	if len(defs) != 1 || defs[0].Function.Name != "plugin_probe" {
		t.Fatalf("plugin tool definitions = %+v", defs)
	}
	provider, ok := reg.Get("plugin_probe")
	if !ok {
		t.Fatal("plugin tool missing from runtime registry")
	}
	text, err := provider.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("probe output %q: %v", text, err)
	}
	if result["cwd"] != repo || result["env"] != installed.Dir || result["arg"] != installed.Dir || result["literal"] != "$OCR_NO_EXPAND" {
		t.Fatalf("runtime context = %v", result)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("enabled MCP subprocess did not run")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	disabled, _, err := rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	clients, err = initRuntimeMCPClients(context.Background(), nil, disabled, tool.NewRegistry(), repo, "test")
	if err != nil || len(clients) != 0 {
		t.Fatalf("disabled clients = %v, %v", clients, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("disabled plugin started subprocess: %v", err)
	}
	if clients, err := initRuntimeMCPClients(context.Background(), nil, nil, tool.NewRegistry(), repo, "test"); err != nil || len(clients) != 0 {
		t.Fatalf("empty runtime = %v, %v", clients, err)
	}
}

func TestPluginPreviewsAndRuleInspectionNeverStartMCP(t *testing.T) {
	store := pluginRuntimeHome(t)
	marker := filepath.Join(t.TempDir(), "started")
	installRuntimePlugin(t, store, marker, true)
	repo := initRulesCheckTestRepo(t)
	gitCommitFile(t, repo, "main.go", "package main\n", "initial")
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n// changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	setRulesCheckRepo(t, repo)
	output := captureDelegateStdout(t, func() {
		if err := executeReview(reviewOptions{repoDir: repo, preview: true, outputFormat: "json"}); err != nil {
			t.Fatal(err)
		}
		if err := executeScan(scanOptions{repoDir: repo, preview: true, outputFormat: "json"}); err != nil {
			t.Fatal(err)
		}
		if err := executeDelegateRule(delegateOptions{repoDir: repo}, []string{"main.go"}); err != nil {
			t.Fatal(err)
		}
		if err := runRulesCheck("main.go"); err != nil {
			t.Fatal(err)
		}
	})
	if !bytes.Contains(output, []byte("Runtime plugin rule marker")) || !bytes.Contains(output, []byte("Plugin: runtime@1.0")) {
		t.Fatalf("rule inspection missed plugin contribution: %s", output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("preview or rule inspection started MCP: %v", err)
	}
}

func TestPluginMCPRuntimeErrorsAreVisible(t *testing.T) {
	store := pluginRuntimeHome(t)
	marker := filepath.Join(t.TempDir(), "started")
	installed := installRuntimePlugin(t, store, marker, true)
	repo := t.TempDir()
	r, _, err := rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{MCPServers: map[string]MCPServerConfig{"plugin/runtime/tools": {Command: "user-command"}}}
	if clients, err := initRuntimeMCPClients(context.Background(), cfg, r, tool.NewRegistry(), repo, "test"); err == nil || len(clients) != 0 {
		t.Fatalf("runtime collision = %v, %v", clients, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("collision started plugin MCP: %v", err)
	}
	installed.Manifest.MCPServers["tools"] = plugin.MCPServer{Command: "ocr-nonexistent-plugin-command"}
	data, _ := json.Marshal(installed.Manifest)
	if err := os.WriteFile(filepath.Join(installed.Source, plugin.ManifestFilename), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateWithOptions(context.Background(), "runtime", plugin.UpdateOptions{AcceptPermissions: true}); err != nil {
		t.Fatal(err)
	}
	r, _, err = rules.NewResolver(repo, "", rules.ResolverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := captureStderr(t, func() {
		clients, err := initRuntimeMCPClients(context.Background(), nil, r, tool.NewRegistry(), repo, "test")
		if err != nil || len(clients) != 0 {
			t.Fatalf("MCP connection failure policy = %v, %v", clients, err)
		}
	})
	if !strings.Contains(diagnostic, `failed to start MCP server "plugin/runtime/tools"`) {
		t.Fatalf("missing plugin MCP failure diagnostic: %s", diagnostic)
	}
}

func TestReviewAndScanReceivePluginToolsAndRules(t *testing.T) {
	for _, mode := range []string{"review", "scan"} {
		t.Run(mode, func(t *testing.T) {
			store := pluginRuntimeHome(t)
			marker := filepath.Join(t.TempDir(), "started")
			installRuntimePlugin(t, store, marker, true)
			repo := initRulesCheckTestRepo(t)
			gitCommitFile(t, repo, "main.go", "package main\n", "initial")
			if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n// changed\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var sawTool, sawRule bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				mu.Lock()
				sawTool = sawTool || bytes.Contains(data, []byte(`"name":"plugin_probe"`))
				sawRule = sawRule || bytes.Contains(data, []byte("Runtime plugin rule marker"))
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if bytes.Contains(data, []byte(`"tools"`)) {
					fmt.Fprint(w, `{"id":"msg","type":"message","role":"assistant","model":"test","content":[{"type":"tool_use","id":"done","name":"task_done","input":{"state":"DONE"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
				} else {
					fmt.Fprint(w, `{"id":"msg","type":"message","role":"assistant","model":"test","content":[{"type":"text","text":"Review the file"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				}
			}))
			defer server.Close()
			t.Setenv("OCR_LLM_URL", server.URL+"/v1/messages")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "test")
			t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
			t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
			captureDelegateStdout(t, func() {
				var err error
				if mode == "review" {
					err = executeReview(reviewOptions{repoDir: repo, outputFormat: "json", audience: "agent", concurrency: 1, maxGitProcs: 2, noFilter: true})
				} else {
					err = executeScan(scanOptions{repoDir: repo, paths: "main.go", outputFormat: "json", audience: "agent", concurrency: 1, maxGitProcs: 2, noPlan: true, noDedup: true, noSummary: true})
				}
				if err != nil {
					t.Fatal(err)
				}
			})
			mu.Lock()
			defer mu.Unlock()
			if !sawTool || !sawRule {
				t.Fatalf("%s requests: plugin tool=%v, plugin rule=%v", mode, sawTool, sawRule)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("%s did not start plugin MCP: %v", mode, err)
			}
		})
	}
}

func TestReviewAndScanContextCancellationWithPlugins(t *testing.T) {
	for _, mode := range []string{"review", "scan"} {
		t.Run(mode, func(t *testing.T) {
			store := pluginRuntimeHome(t)
			marker := filepath.Join(t.TempDir(), "started")
			installRuntimePlugin(t, store, marker, true)
			repo := initRulesCheckTestRepo(t)
			gitCommitFile(t, repo, "main.go", "package main\n", "initial")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"msg","type":"message","role":"assistant","model":"test","content":[{"type":"tool_use","id":"done","name":"task_done","input":{"state":"DONE"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
			}))
			defer server.Close()
			t.Setenv("OCR_LLM_URL", server.URL+"/v1/messages")
			t.Setenv("OCR_LLM_TOKEN", "test-token")
			t.Setenv("OCR_LLM_MODEL", "test")
			t.Setenv("OCR_LLM_PROTOCOL", "anthropic")
			t.Setenv("OCR_LLM_AUTH_HEADER", "x-api-key")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			reportPath := filepath.Join(t.TempDir(), "report.json")
			captureDelegateStdout(t, func() {
				var err error
				if mode == "review" {
					err = executeReviewContext(ctx, reviewOptions{repoDir: repo, commit: "HEAD", outputFormat: "json", outputPath: reportPath, audience: "agent", concurrency: 1, maxGitProcs: 2, noFilter: true})
				} else {
					err = executeScanContext(ctx, scanOptions{repoDir: repo, paths: "main.go", outputFormat: "json", audience: "agent", concurrency: 1, maxGitProcs: 2, noPlan: true, noDedup: true, noSummary: true})
				}
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("%s lost caller cancellation: %v", mode, err)
				}
			})
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("canceled %s started plugin MCP: %v", mode, err)
			}
			if mode == "review" {
				data, err := os.ReadFile(reportPath)
				if err != nil {
					t.Fatal(err)
				}
				var result jsonOutput
				if err := json.Unmarshal(data, &result); err != nil || result.Manifest == nil || result.SessionID == "" {
					t.Fatalf("canceled review report lost: %+v, %v", result, err)
				}
			}
		})
	}
}
