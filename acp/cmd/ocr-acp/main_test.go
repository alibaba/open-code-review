// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/llmresolve"
	acp "github.com/coder/acp-go-sdk"
)

func TestParserConfigurationIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		opts                  llmresolve.Options
		wantError, wantParser bool
	}{
		{name: "absent"},
		{name: "partial", opts: llmresolve.Options{Provider: "openai"}, wantError: true},
		{name: "invalid", opts: llmresolve.Options{Provider: "bad", Model: "m", APIKey: "secret"}, wantError: true},
		{name: "valid", opts: llmresolve.Options{Provider: "openai", Model: "m", APIKey: "secret"}, wantParser: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.Getenv = func(string) string { return "" }
			p, err := configuredParser(tc.opts)
			if (err != nil) != tc.wantError || (p != nil) != tc.wantParser {
				t.Fatalf("parser=%v error=%v", p != nil, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("credential leaked")
			}
		})
	}
}

func TestACPProcessHelper(t *testing.T) {
	if os.Getenv("OCR_ACP_TEST_HELPER") != "1" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("ocr-acp", flag.ExitOnError)
	os.Args = []string{"ocr-acp", "--ocr-binary", os.Getenv("OCR_ACP_TEST_BINARY")}
	main()
	os.Exit(0)
}

func TestDisconnectReapsOCRBeforeAdapterExit(t *testing.T) {
	for _, mode := range []string{"EOF", "SIGTERM"} {
		t.Run(mode, func(t *testing.T) { testAdapterCleanup(t, mode) })
	}
}

func testAdapterCleanup(t *testing.T, mode string) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process control")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	binary := filepath.Join(dir, "ocr")
	script := "#!/bin/sh\ntrap 'echo cleaned > cleaned; exit 0' INT TERM\necho READY >&2\nwhile :; do sleep 0.1; done\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestACPProcessHelper$")
	cmd.Env = append(os.Environ(), "OCR_ACP_TEST_HELPER=1", "OCR_ACP_TEST_BINARY="+binary)
	var logs bytes.Buffer
	cmd.Stderr = &logs
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	ready := make(chan struct{}, 1)
	client := acp.NewConnection(func(_ context.Context, method string, raw json.RawMessage) (any, *acp.RequestError) {
		if method == acp.ClientMethodSessionUpdate && strings.Contains(string(raw), "READY") {
			select {
			case ready <- struct{}{}:
			default:
			}
		}
		return nil, nil
	}, input, output)
	init, err := acp.SendRequest[acp.InitializeResponse](client, ctx, acp.AgentMethodInitialize, acp.InitializeRequest{ProtocolVersion: 1})
	if err != nil || init.ProtocolVersion != 1 {
		t.Fatalf("initialize: %v %+v", err, init)
	}
	session, err := acp.SendRequest[acp.NewSessionResponse](client, ctx, acp.AgentMethodSessionNew, acp.NewSessionRequest{Cwd: dir, McpServers: []acp.McpServer{}})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = acp.SendRequest[acp.PromptResponse](client, ctx, acp.AgentMethodSessionPrompt, acp.PromptRequest{SessionId: session.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock("/review")}})
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("OCR readiness timeout")
	}
	if mode == "SIGTERM" {
		err = cmd.Process.Signal(syscall.SIGTERM)
	} else {
		err = input.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatalf("adapter exit: %v; %s", err, logs.String())
	}
	if _, err = os.Stat(filepath.Join(dir, "cleaned")); err != nil {
		t.Fatalf("adapter exited before OCR cleanup: %v; %s", err, logs.String())
	}
}
