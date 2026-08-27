// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/session"
)

func TestLoadLLMRuntime_RawHolderFollowsSwitch(t *testing.T) {
	setTestHome(t, t.TempDir())
	t.Setenv("OCR_LLM_URL", "https://api.example.test/v1")
	t.Setenv("OCR_LLM_TOKEN", "tok-123")
	t.Setenv("OCR_LLM_MODEL", "test-model")

	tpl := loadTestTemplate(t)
	rt, err := loadLLMRuntime(tpl, "", llm.ResolveOptions{})
	if err != nil {
		t.Fatalf("loadLLMRuntime error: %v", err)
	}
	if rt.RawHolder != nil {
		t.Error("raw holder created with OCR_RAW_LOGGING unset")
	}

	t.Setenv("OCR_RAW_LOGGING", "1")
	tpl = loadTestTemplate(t)
	rt, err = loadLLMRuntime(tpl, "", llm.ResolveOptions{})
	if err != nil {
		t.Fatalf("loadLLMRuntime error: %v", err)
	}
	if rt.RawHolder == nil {
		t.Fatal("raw holder not created with OCR_RAW_LOGGING=1")
	}
}

func TestBindRawWriter_NilHolderIsNoop(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	sess := session.New(filepath.Join(home, "repo"), "", "m", session.SessionOptions{})

	closer := bindRawWriter(nil, filepath.Join(home, "repo"), sess)
	closer() // must not panic

	rawDir := filepath.Join(home, ".opencodereview", "raw")
	if _, err := os.Stat(rawDir); !os.IsNotExist(err) {
		t.Errorf("raw dir must not exist with capture off, stat err = %v", err)
	}
}

func TestBindRawWriter_OpensSessionRawFile(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	repoDir := filepath.Join(home, "repo")
	sess := session.New(repoDir, "", "m", session.SessionOptions{})

	holder := llm.NewRawHolder()
	closer := bindRawWriter(holder, repoDir, sess)
	defer closer()

	rawDir := filepath.Join(home, ".opencodereview", "raw")
	want := sess.SessionID + ".jsonl"
	found := false
	_ = filepath.WalkDir(rawDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == want {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("raw capture file %s not created under %s", want, rawDir)
	}
}

func TestBindRawWriter_OpenFailureIsNoop(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	// Point HOME at a regular file so the raw tree cannot be created.
	fileHome := filepath.Join(home, "notadir")
	if err := os.WriteFile(fileHome, []byte("x"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	setTestHome(t, fileHome)
	repoDir := filepath.Join(home, "repo")
	sess := session.New(repoDir, "", "m", session.SessionOptions{})

	holder := llm.NewRawHolder()
	closer := bindRawWriter(holder, repoDir, sess)
	closer() // must not panic; raw capture stays degraded, review continues
}
