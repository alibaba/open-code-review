// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/rules"
	"github.com/alibaba/open-code-review/internal/plugin"
)

func TestLocalPluginCommandsAndRules(t *testing.T) {
	home, source := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for name, content := range map[string]string{
		plugin.ManifestFilename: `{"schema_version":1,"id":"checklist","name":"Checklist","version":"1","rules":["rules.json"]}`,
		"rules.json":            `{"rules":[{"path":"**/*.go","rule":"Check transaction boundaries."}]}`,
	} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := newPluginCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.ExecuteContext(context.Background())
		return out.String(), err
	}
	for _, args := range [][]string{{"list", "--json"}, {"install", source, "--json"}, {"show", "checklist", "--json"}} {
		out, err := run(args...)
		if err != nil || !json.Valid([]byte(out)) {
			t.Fatalf("%v: %s, %v", args, out, err)
		}
	}
	loaded := func() string {
		t.Helper()
		r, _, err := rules.NewResolver(t.TempDir(), "", rules.ResolverOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return r.Resolve("main.go")
	}
	if strings.Contains(loaded(), "Check transaction boundaries.") {
		t.Fatal("installation enabled rules implicitly")
	}
	if out, err := run("enable", "checklist"); err != nil || !strings.Contains(out, "enabled") {
		t.Fatalf("enable: %s, %v", out, err)
	}
	if !strings.Contains(loaded(), "Check transaction boundaries.") {
		t.Fatal("enabled rules were not applied")
	}
	if out, err := run("list"); err != nil || !strings.Contains(out, "checklist") {
		t.Fatalf("list: %s, %v", out, err)
	}
	if _, err := run("disable", "checklist"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(loaded(), "Check transaction boundaries.") {
		t.Fatal("disabled rules were applied")
	}
	if out, err := run("remove", "checklist", "--json"); err != nil || !strings.Contains(out, `"removed": "checklist"`) {
		t.Fatalf("remove: %s, %v", out, err)
	}
	if _, err := run("install", source, "--enable"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded(), "Check transaction boundaries.") {
		t.Fatal("--enable was ignored")
	}
	for _, args := range [][]string{{"show", "missing"}, {"install"}, {"list", "extra"}, {"install", "https://example.com/package.zip"}, {"update", "checklist"}, {"market"}, {"enable", "missing"}, {"remove", "missing"}} {
		if _, err := run(args...); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}

func TestPluginTextRemovesTerminalControl(t *testing.T) {
	if got := pluginText("\x1b[31mName\x1b[0m\r\nnext\x00\tpart"); got != "Name next part" {
		t.Fatalf("unsafe text: %q", got)
	}
}

func TestRulesCheckSanitizesPluginMetadata(t *testing.T) {
	home, source := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	setRulesCheckRepo(t, initRulesCheckTestRepo(t))
	writeRulesCheckTestFile(t, source, plugin.ManifestFilename, `{"schema_version":1,"id":"checklist","name":"Checklist","version":"\u001b[31m1\u001b[0m","rules":["rules.json"]}`)
	writeRulesCheckTestFile(t, source, "rules.json", `{"rules":[{"path":"**/*.go","rule":"Check transactions."}]}`)
	store, err := plugin.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Install(context.Background(), source, true); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runRulesCheck("main.go"); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "\x1b") || !strings.Contains(out, "Plugin: checklist@1 ") {
		t.Fatalf("unsafe plugin metadata output: %q", out)
	}
}
