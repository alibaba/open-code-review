// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/acp/internal/orchestrator"
	acp "github.com/coder/acp-go-sdk"
)

func TestPromptContentResources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file with space.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{" leading.go", "trailing.go "} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("package sample\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := promptContent(dir, []acp.ContentBlock{acp.ResourceLinkBlock(name, (&url.URL{Scheme: "file", Path: p}).String())}); err == nil {
			t.Fatalf("accepted whitespace path %q", name)
		}
	}
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	text, paths, err := promptContent(dir, []acp.ContentBlock{acp.TextBlock("/scan"), acp.TextBlock("--no-plan"), acp.ResourceLinkBlock("untrusted name", uri)})
	if err != nil || text != "/scan\n--no-plan" || len(paths) != 1 || paths[0] != "file with space.go" {
		t.Fatalf("%q %v %v", text, paths, err)
	}
	for _, bad := range []string{"https://example.com/file", "file://remote/file", "file:///missing", uri + "?query", uri + "#fragment", "file:///bad%zz", "file:relative", "file://user@localhost/file", "file:///" + strings.Repeat("a", 8192)} {
		if _, _, err := promptContent(dir, []acp.ContentBlock{acp.ResourceLinkBlock("x", bad)}); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	for _, blocks := range [][]acp.ContentBlock{{{}}, {acp.TextBlock(strings.Repeat("a", 65537))}, make([]acp.ContentBlock, 129)} {
		if _, _, err := promptContent(dir, blocks); err == nil {
			t.Fatal("accepted unsupported or oversized input")
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.go")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := promptContent(dir, []acp.ContentBlock{acp.ResourceLinkBlock("link", (&url.URL{Scheme: "file", Path: link}).String())}); err == nil {
		t.Fatal("accepted symlink escape")
	}
}

type captureRunner struct{ requests []orchestrator.Request }

func (r *captureRunner) Run(ctx context.Context, request orchestrator.Request) (<-chan orchestrator.Event, <-chan orchestrator.Outcome) {
	r.requests = append(r.requests, request)
	return fakeRunner{}.Run(ctx, request)
}

func TestResourceRouting(t *testing.T) {
	dir := testGitDir(t)
	path := filepath.Join(dir, "file.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := acp.ResourceLinkBlock("sample", (&url.URL{Scheme: "file", Path: path}).String())
	runner := &captureRunner{}
	agent := NewAgent("ocr", runner)
	session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		text string
		stop acp.StopReason
		runs int
	}{
		{"", acp.StopReasonEndTurn, 0},
		{"/review", acp.StopReasonEndTurn, 0},
		{"/scan --path file.go", acp.StopReasonEndTurn, 0},
		{"/scan", acp.StopReasonEndTurn, 1},
	} {
		response, err := agent.Prompt(context.Background(), acp.PromptRequest{SessionId: session.SessionId, Prompt: []acp.ContentBlock{acp.TextBlock(tc.text), link}})
		if err != nil || response.StopReason != tc.stop || len(runner.requests) != tc.runs {
			t.Fatalf("%q: %+v %v runs=%d", tc.text, response, err, len(runner.requests))
		}
	}
	if !strings.Contains(strings.Join(runner.requests[0].Args, " "), "--path file.go") {
		t.Fatal(runner.requests)
	}
	r, err := agent.Prompt(context.Background(), acp.PromptRequest{SessionId: session.SessionId, Prompt: []acp.ContentBlock{{}}})
	if err != nil || r.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("unsupported: %+v %v", r, err)
	}
}

func TestCommaResourcePathGetsVisibleRejectionWithoutRunningOCR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a,b.go")
	if err := os.WriteFile(path, []byte("package sample\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &captureRunner{}
	agent := NewAgent("ocr", runner)
	session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: dir})
	if err != nil {
		t.Fatal(err)
	}
	sink := &discoveryRecorder{}
	agent.SetAgentConnection(sink)
	response, err := agent.Prompt(context.Background(), acp.PromptRequest{
		SessionId: session.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("/scan"), acp.ResourceLinkBlock("sample", (&url.URL{Scheme: "file", Path: path}).String())},
	})
	if len(runner.requests) != 0 {
		t.Fatal("invalid resource started OCR")
	}
	if err != nil || response.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	meta, _ := response.Meta["ocr"].(map[string]any)
	if meta["kind"] != "rejected" {
		t.Fatalf("missing rejection metadata: %+v", response)
	}
	if len(sink.updates) != 1 || sink.updates[0].SessionId != session.SessionId {
		t.Fatalf("updates=%+v", sink.updates)
	}
	chunk := sink.updates[0].Update.AgentMessageChunk
	if chunk == nil || chunk.Content.Text == nil || !strings.Contains(chunk.Content.Text.Text, "path must not contain a comma") {
		t.Fatalf("missing actionable guidance: %+v", chunk)
	}
}

type discoveryRecorder struct {
	updates []acp.SessionNotification
	err     error
}

func (r *discoveryRecorder) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	r.updates = append(r.updates, n)
	return r.err
}

func TestNewSessionDoesNotNotifyBeforeResponse(t *testing.T) {
	agent := NewAgent("ocr", fakeRunner{})
	sink := &discoveryRecorder{err: errors.New("must not send before response")}
	agent.SetAgentConnection(sink)
	s, err := agent.NewSession(context.Background(), acp.NewSessionRequest{Cwd: t.TempDir()})
	if err != nil || s.SessionId == "" || len(sink.updates) != 0 {
		t.Fatalf("premature discovery: session=%+v updates=%+v error=%v", s, sink.updates, err)
	}
}
