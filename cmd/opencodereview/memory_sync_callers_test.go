// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/knowledge"
	"github.com/alibaba/open-code-review/internal/llm"
)

func TestMemorySyncAcceptedPythonCaller(t *testing.T) {
	var extractions atomic.Int32
	ordinary := newFakeLLM()
	startFakeLLM(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if !bytes.Contains(body, []byte("Extract repository knowledge")) {
			r.Body = io.NopCloser(bytes.NewReader(body))
			ordinary.ServeHTTP(w, r)
			return
		}
		extractions.Add(1)
		for _, s := range []string{"result_use", "value = read()", "write(value)", "return accepted_value"} {
			if !bytes.Contains(body, []byte(s)) {
				t.Errorf("missing accepted evidence %q", s)
			}
		}
		if bytes.Contains(body, []byte("proposed_value")) {
			t.Error("proposed source in discovery")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sync","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"{\"records\":[]}"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	repo := initResumeRepo(t)
	if err := os.Mkdir(filepath.Join(repo, "app"), 0700); err != nil {
		t.Fatal(err)
	}
	const target = "app/source.py"
	commitFile(t, repo, target, "def read():\n    return accepted_value\n", "Define the source")
	const deep = "app/internal/jobs/dispatch"
	if err := os.MkdirAll(filepath.Join(repo, deep), 0700); err != nil {
		t.Fatal(err)
	}
	// These nearby modules exceed the Git path budget if the deeper changed
	// directory gets all the slots before the other changed module's siblings.
	for i := range 180 {
		file := fmt.Sprintf("%s/component_with_a_long_repository_relative_name_%03d.py", deep, i)
		if err := os.WriteFile(filepath.Join(repo, file), []byte("enabled = True\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	const other = deep + "/task.py"
	commitFile(t, repo, other, "def poll():\n    return 2\n", "Define the nested tasks")
	commitFile(t, repo, "app/consumer.py", "from .source import read\n\ndef save():\n    value = read()\n    write(value)\n", "Persist the value")
	base := revParse(t, repo, "HEAD")
	if err := os.WriteFile(filepath.Join(repo, other), []byte("def poll():\n    return 3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commitFile(t, repo, target, "def read():\n    return proposed_value\n", "Change the source")
	head := revParse(t, repo, "HEAD")
	db := filepath.Join(t.TempDir(), "memory.db")
	cc, err := loadCommonContext(repo, "", base, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := loadLLMRuntime(cc.Template, llm.ResolveOptions{Context: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	prior := struct {
		Version, Revision, Provider, Model string
		Live                               bool
		Options                            knowledge.DiscoverOptions
		Sources                            []string
		FocusedCallerVersion               string `json:",omitempty"`
	}{"business-discovery-v2", base, rt.Provider, rt.Model, false, knowledge.DiscoverOptions{Paths: []string{target}, FocusLines: map[string][]int{target: {2}}, FileFilter: cc.FileFilter}, nil, ""}
	store, err := openMemory(t.Context(), db, repo, cc.GitRunner, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, algorithm := range []string{"", "python-relative-calls-v1", "python-relative-calls-v2", "python-relative-calls-v3"} {
		prior.FocusedCallerVersion = algorithm
		old, err := json.Marshal(prior)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveDiscovery(t.Context(), knowledge.Digest(old), nil); err != nil {
			t.Fatal(err)
		}
	}
	store.Close()
	for _, want := range []bool{false, true} {
		var stdout strings.Builder
		cmd := newMemoryCommand()
		cmd.SetOut(&stdout)
		cmd.SetArgs([]string{"--repo", repo, "--memory-db", db, "sync", "--ref", base, "--at", target + ":2"})
		var report memorySyncReport
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil || report.Cached != want {
			t.Fatalf("focused sync: %+v, %v (wanted cached %t)", report, err, want)
		}
	}
	if err := runReview([]string{"--repo", repo, "--from", base, "--to", head, "--memory-db", db, "--memory-sync", "--audience", "agent", "--format", "json", "--output", filepath.Join(t.TempDir(), "review.json")}); err != nil {
		t.Fatal(err)
	}
	if n := extractions.Load(); n != 2 {
		t.Fatalf("wanted one discovery request per distinct native entry point, got %d", n)
	}
}
