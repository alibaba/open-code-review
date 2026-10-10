// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/acp/internal/testutil"
)

func TestRealBinaryUnreadStdoutExits(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	adapter := filepath.Join(dir, testutil.ExeName("ocr-acp"))
	args := []string{"build", "-o", adapter, "."}
	if runtime.GOOS != "windows" {
		args = []string{"build", "-race", "-o", adapter, "."}
	}
	build := exec.Command("go", args...)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	ocr := testutil.Install(t, dir, "ocr", &testutil.Config{HugeMessage: 1 << 20})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, adapter, "--ocr-binary", ocr)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "OCR_ACP_PARSER_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	var logs bytes.Buffer
	cmd.Stderr = &logs
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = in.Close(); _ = out.Close() })
	enc, dec := json.NewEncoder(in), json.NewDecoder(out)
	request := func(id int, method string, params any) map[string]any {
		t.Helper()
		if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		var response map[string]any
		for {
			if err := dec.Decode(&response); err != nil {
				t.Fatal(err)
			}
			if response["id"] != nil {
				break
			}
			response = nil
		}
		if response["error"] != nil {
			t.Fatalf("request failed: %+v", response)
		}
		return response
	}
	request(1, "initialize", map[string]any{"protocolVersion": 1})
	s := request(2, "session/new", map[string]any{"cwd": dir, "mcpServers": []any{}})
	id := s["result"].(map[string]any)["sessionId"]
	start := time.Now()
	if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "session/prompt", "params": map[string]any{"sessionId": id, "prompt": []any{map[string]any{"type": "text", "text": "/review"}}}}); err != nil {
		t.Fatal(err)
	}
	// Keep stdin and stdout open, and never read stdout again. Neither EOF nor
	// restoring a reader may be needed to make the process exit.
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit: %v; %s", err, logs.String())
	}
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Fatalf("blocked for %s", elapsed)
	}
}
