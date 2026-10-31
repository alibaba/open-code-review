// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setTestHome points os.UserHomeDir() at dir. Setting HOME alone is not enough on
// Windows, where USERPROFILE wins; that variable is a no-op elsewhere.
func setTestHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "empty means local", cfg: Config{}},
		{name: "local", cfg: Config{Type: TypeLocal}},
		{name: "filesystem with path", cfg: Config{Type: TypeFilesystem, Path: "/mnt/sessions"}},
		{
			name:    "filesystem without path",
			cfg:     Config{Type: TypeFilesystem},
			wantErr: "requires path",
		},
		{
			name: "s3 complete",
			cfg: Config{Type: TypeS3, S3: S3Config{
				Bucket: "b", Region: "us-east-1", AccessKeyID: "a", SecretAccessKey: "s", PathStyle: true,
			}},
		},
		{
			name:    "s3 without bucket",
			cfg:     Config{Type: TypeS3, S3: S3Config{Region: "us-east-1", AccessKeyID: "a", SecretAccessKey: "s"}},
			wantErr: "s3.bucket",
		},
		{
			name:    "s3 without region",
			cfg:     Config{Type: TypeS3, S3: S3Config{Bucket: "b", AccessKeyID: "a", SecretAccessKey: "s"}},
			wantErr: "s3.region",
		},
		{
			name:    "s3 without credentials",
			cfg:     Config{Type: TypeS3, S3: S3Config{Bucket: "b", Region: "us-east-1"}},
			wantErr: "s3.access_key_id",
		},
		{
			name:    "unknown type",
			cfg:     Config{Type: "gcs"},
			wantErr: "unknown storage type",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate returned no error, want one containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestOpen(t *testing.T) {
	local, err := Open(Config{Type: TypeLocal})
	if err != nil {
		t.Fatalf("Open local: %v", err)
	}
	if local != nil {
		t.Error("a local configuration must not produce a backend: nothing to mirror to")
	}

	defaulted, err := Open(Config{})
	if err != nil || defaulted != nil {
		t.Errorf("an absent type must behave as local, got backend=%v err=%v", defaulted, err)
	}

	dir := t.TempDir()
	fsBackend, err := Open(Config{Type: TypeFilesystem, Path: dir})
	if err != nil {
		t.Fatalf("Open filesystem: %v", err)
	}
	if fsBackend.Name() != TypeFilesystem {
		t.Errorf("Name = %q", fsBackend.Name())
	}
	ctx := context.Background()
	if err := fsBackend.Put(ctx, "sessions/repo/id.jsonl", []byte("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sessions", "repo", "id.jsonl")); err != nil {
		t.Errorf("object not written under the configured root: %v", err)
	}

	prefixed, err := Open(Config{Type: TypeFilesystem, Path: dir, Prefix: "/prod/"})
	if err != nil {
		t.Fatalf("Open with prefix: %v", err)
	}
	if err := prefixed.Put(ctx, "sessions/repo/id2.jsonl", []byte("y")); err != nil {
		t.Fatalf("Put with prefix: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "prod", "sessions", "repo", "id2.jsonl")); err != nil {
		t.Errorf("prefix not applied to the object path: %v", err)
	}

	s3Backend, err := Open(Config{Type: TypeS3, S3: S3Config{
		Bucket: "b", Region: "us-east-1", AccessKeyID: "a", SecretAccessKey: "s",
	}})
	if err != nil {
		t.Fatalf("Open s3: %v", err)
	}
	if s3Backend.Name() != TypeS3 {
		t.Errorf("Name = %q", s3Backend.Name())
	}

	if _, err := Open(Config{Type: "gcs"}); err == nil {
		t.Error("Open accepted an unknown storage type")
	}
}

func TestKeyJoinsSegments(t *testing.T) {
	cases := []struct {
		segments []string
		want     string
	}{
		{[]string{"sessions", "repo", "id.jsonl"}, "sessions/repo/id.jsonl"},
		{[]string{"", "sessions", "", "repo"}, "sessions/repo"},
		{[]string{"/sessions/", "/repo/"}, "sessions/repo"},
		{[]string{"sessions"}, "sessions"},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := Key(tc.segments...); got != tc.want {
			t.Errorf("Key(%v) = %q, want %q", tc.segments, got, tc.want)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)

	// An install that never configured storage has no file at all.
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig without a file: %v", err)
	}
	if cfg.Type != "" {
		t.Errorf("Type = %q, want the zero value for an unconfigured install", cfg.Type)
	}
	backend, err := Open(cfg)
	if err != nil || backend != nil {
		t.Errorf("unconfigured storage opened a backend: %v %v", backend, err)
	}

	configPath := filepath.Join(home, ".opencodereview", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	document := map[string]any{
		"provider": "anthropic",
		"storage": map[string]any{
			"type":   "s3",
			"prefix": "prod",
			"s3": map[string]any{
				"endpoint":          "https://s3.internal",
				"bucket":            "ocr",
				"region":            "us-west-2",
				"access_key_id":     "id",
				"secret_access_key": "secret",
			},
		},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err = LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Type != TypeS3 || cfg.Prefix != "prod" || cfg.S3.Bucket != "ocr" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !cfg.S3.PathStyle {
		t.Error("an absent path_style must default to true, which S3-compatible servers need")
	}

	backend, err = Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	prefixed, ok := backend.(*prefixedBackend)
	if !ok {
		t.Fatalf("backend = %T, want the prefix wrapper", backend)
	}
	if prefixed.prefix != "prod/" {
		t.Errorf("prefix = %q, want prod/", prefixed.prefix)
	}

	// The environment wins over the file, which is what makes a CI run able to
	// point at its own bucket without rewriting the checked-in configuration.
	t.Setenv(EnvS3Bucket, "ci-bucket")
	t.Setenv(EnvS3PathStyle, "false")
	cfg, err = LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig with env: %v", err)
	}
	if cfg.S3.Bucket != "ci-bucket" {
		t.Errorf("bucket = %q, want the environment override", cfg.S3.Bucket)
	}
	if cfg.S3.PathStyle {
		t.Error("path_style = true, want the environment override to win")
	}
}

func TestLoadConfigRejectsBrokenInput(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	configPath := filepath.Join(home, "config.json")
	if err := os.WriteFile(configPath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadConfig(configPath); err == nil {
		t.Error("LoadConfig accepted a malformed configuration file")
	}

	t.Setenv(EnvS3PathStyle, "sometimes")
	if _, err := LoadConfig(filepath.Join(home, "absent.json")); err == nil {
		t.Error("LoadConfig accepted a non-boolean path_style")
	}
}

func TestConfigPathUsesHome(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := filepath.Join(home, ".opencodereview", "config.json")
	if got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
}

func TestFileConfigValidateAndConfig(t *testing.T) {
	section := FileConfig{Type: "s3", S3: FileS3Config{Bucket: "b", Region: "r", AccessKeyID: "a", SecretAccessKey: "s"}}
	if err := section.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	pathStyle := false
	section = FileConfig{Type: "s3", S3: FileS3Config{
		Bucket: "b", Region: "r", AccessKeyID: "a", SecretAccessKey: "s", PathStyle: &pathStyle,
	}}
	if section.Config().S3.PathStyle {
		t.Error("an explicit path_style=false must be kept")
	}

	partial := FileConfig{Type: "s3"}
	if err := partial.Validate(); err == nil {
		t.Error("Validate accepted an incomplete s3 section")
	}
}

func TestBackendErrorsAreDistinguishable(t *testing.T) {
	// Callers switch on ErrNotExist to decide between "no such session" and a
	// transport failure, so a wrapped missing object must satisfy errors.Is.
	fileBackend := newFileBackend(t.TempDir())
	_, err := fileBackend.Get(context.Background(), "sessions/repo/missing.jsonl")
	if !errors.Is(err, ErrNotExist) {
		t.Errorf("Get for a missing object: err = %v, want ErrNotExist", err)
	}
}
