// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func processPlugin(t *testing.T, mode string) Installed {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Installed{Manifest: Manifest{SchemaVersion: 1, ID: "example", Name: "Example", Version: "1", Review: &ReviewContribution{Protocol: "stdio-v1", Command: executable, Args: []string{"-test.run=^TestReviewProcess$", "--", "review-helper", mode}, Stages: []string{"inspect"}}}, Dir: t.TempDir()}
}

func TestReviewContributionValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ReviewContribution)
	}{
		{"protocol", func(r *ReviewContribution) { r.Protocol = "unknown" }},
		{"command", func(r *ReviewContribution) { r.Command = " " }},
		{"empty stages", func(r *ReviewContribution) { r.Stages = nil }},
		{"duplicates", func(r *ReviewContribution) { r.Stages = []string{"inspect", "inspect"} }},
		{"invalid stage", func(r *ReviewContribution) { r.Stages = []string{"../inspect"} }},
		{"oversized stages", func(r *ReviewContribution) { r.Stages = make([]string, 65) }},
		{"nul", func(r *ReviewContribution) { r.Args = []string{"\x00"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := processPlugin(t, "ok").Manifest
			tc.edit(m.Review)
			data, _ := json.Marshal(m)
			if _, err := ParseManifest(data); err == nil {
				t.Fatal("invalid contribution accepted")
			}
		})
	}
	m := processPlugin(t, "ok").Manifest
	data, _ := json.Marshal(m)
	if _, err := ParseManifest(data); err != nil {
		t.Fatal(err)
	}
	legacy, err := ParseManifest([]byte(`{"schema_version":1,"id":"x","name":"X","version":"1","review":{"engine":"old","prompt":"old.md"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReviewCommand(Installed{Manifest: legacy}); err == nil {
		t.Fatal("retired engine is executable")
	}
}

func TestRetiredReviewCanBeListedAndUpdated(t *testing.T) {
	s := NewStore(t.TempDir())
	source := t.TempDir()
	old := `{"schema_version":1,"id":"example","name":"Example","version":"0.1","review":{"engine":"old","prompt":"old.md"}}`
	if err := os.WriteFile(filepath.Join(source, ManifestFilename), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Install(context.Background(), source, false); err != nil {
		t.Fatal(err)
	}
	items, err := s.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("list=%+v error=%v", items, err)
	}
	if _, _, err := ReviewCommand(items[0]); err == nil {
		t.Fatal("retired review can run")
	}
	newManifest := processPlugin(t, "ok").Manifest
	newManifest.Version = "0.2"
	data, _ := json.Marshal(newManifest)
	if err := os.WriteFile(filepath.Join(source, ManifestFilename), data, 0600); err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateWithOptions(context.Background(), "example", UpdateOptions{AcceptPermissions: true})
	if err != nil || updated.Manifest.Version != "0.2" || updated.Enabled {
		t.Fatalf("update=%+v error=%v", updated, err)
	}
	if _, _, err := ReviewCommand(updated); err != nil {
		t.Fatal(err)
	}
}

func TestReviewCommandAndNonExecutingInstall(t *testing.T) {
	p := processPlugin(t, "exit")
	p.Manifest.Review.Command = "${plugin_dir}/bin/${os}-${arch}/example${exe}"
	p.Manifest.Review.Args = []string{"--file", "${plugin_dir}/policy.md"}
	data, _ := json.Marshal(p.Manifest)
	if err := os.WriteFile(filepath.Join(p.Dir, ManifestFilename), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectPackage(context.Background(), p.Dir); err != nil {
		t.Fatal(err)
	}
	command, args, err := ReviewCommand(p)
	if err != nil || !strings.Contains(command, runtime.GOOS+"-"+runtime.GOARCH) || strings.Contains(command, "${") || args[1] != p.Dir+"/policy.md" {
		t.Fatalf("command=%q args=%v error=%v", command, args, err)
	}
	p.Manifest.Review = nil
	if _, _, err := ReviewCommand(p); err == nil {
		t.Fatal("missing contribution accepted")
	}
}

func TestReviewTransport(t *testing.T) {
	for _, mode := range []string{"ok", "missing-complete", "missing-accepted", "rejected-comments", "invalid-json", "wrong-version", "exit", "large"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p := processPlugin(t, mode)
			c, err := StartReview(ctx, p, p.Dir)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			res, err := c.Call(ReviewRequest{Method: "submit", Stage: "inspect"})
			if mode == "ok" {
				if err != nil || !*res.Accepted || !*res.Complete {
					t.Fatalf("response=%+v error=%v", res, err)
				}
			} else if err == nil {
				t.Fatal("invalid plugin response accepted")
			}
		})
	}
}

func TestReviewCancellationReleasesProcess(t *testing.T) {
	p := processPlugin(t, "hang")
	ctx, cancel := context.WithCancel(context.Background())
	c, err := StartReview(ctx, p, p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := c.Call(ReviewRequest{Method: "submit"}); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled call succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled call remained blocked")
	}
	c.Close()
	if c.cmd.ProcessState == nil {
		t.Fatal("child was not reaped")
	}
}

func TestReviewProcess(t *testing.T) {
	i := slices.Index(os.Args, "review-helper")
	if i < 0 {
		return
	}
	mode := os.Args[i+1]
	lines := bufio.NewScanner(os.Stdin)
	for lines.Scan() {
		switch mode {
		case "hang":
			time.Sleep(time.Hour)
		case "exit":
			fmt.Fprintln(os.Stderr, "plugin execution failed")
			os.Exit(2)
		case "invalid-json":
			fmt.Println("invalid")
		case "large":
			fmt.Println(strings.Repeat("x", 4<<20))
		case "wrong-version":
			fmt.Println(`{"version":9}`)
		case "missing-accepted":
			fmt.Println(`{"version":1,"complete":true}`)
		case "missing-complete":
			fmt.Println(`{"version":1,"accepted":true}`)
		case "rejected-comments":
			fmt.Println(`{"version":1,"accepted":false,"feedback":"repair","comments":[{"path":"x"}]}`)
		default:
			fmt.Println(`{"version":1,"accepted":true,"complete":true,"report":{"reviewed":true}}`)
		}
	}
	os.Exit(0)
}
