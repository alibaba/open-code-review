// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/storage"
)

// writeConfigFile writes a config.json into dir's .opencodereview and returns its
// path.
func writeConfigFile(t *testing.T, home, document string) string {
	t.Helper()
	path := filepath.Join(home, ".opencodereview", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestSetConfigValueStorageKeys(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "storage.type", "s3"); err != nil {
		t.Fatalf("set storage.type: %v", err)
	}
	if cfg.Storage == nil || cfg.Storage.Type != storage.TypeS3 {
		t.Fatalf("storage section = %+v", cfg.Storage)
	}
	if err := setConfigValue(cfg, "storage.prefix", "prod"); err != nil {
		t.Fatalf("set storage.prefix: %v", err)
	}
	if err := setConfigValue(cfg, "storage.path", "/mnt/sessions"); err != nil {
		t.Fatalf("set storage.path: %v", err)
	}
	if cfg.Storage.Prefix != "prod" || cfg.Storage.Path != "/mnt/sessions" {
		t.Errorf("storage section = %+v", cfg.Storage)
	}

	for field, want := range map[string]string{
		"s3.endpoint":          "https://s3.internal",
		"s3.bucket":            "ocr",
		"s3.region":            "us-west-2",
		"s3.access_key_id":     "id",
		"s3.secret_access_key": "secret",
		"s3.session_token":     "token",
	} {
		if err := setConfigValue(cfg, "storage."+field, want); err != nil {
			t.Fatalf("set storage.%s: %v", field, err)
		}
	}
	got := cfg.Storage.Config()
	if got.S3.Endpoint != "https://s3.internal" || got.S3.Bucket != "ocr" || got.S3.Region != "us-west-2" {
		t.Errorf("s3 config = %+v", got.S3)
	}
	if got.S3.AccessKeyID != "id" || got.S3.SecretAccessKey != "secret" || got.S3.SessionToken != "token" {
		t.Errorf("s3 credentials = %+v", got.S3)
	}

	// path_style accepts a boolean and the file section is valid as configured.
	if err := setConfigValue(cfg, "storage.s3.path_style", "false"); err != nil {
		t.Fatalf("set storage.s3.path_style: %v", err)
	}
	if cfg.Storage.S3.PathStyle == nil || *cfg.Storage.S3.PathStyle {
		t.Errorf("path_style = %v, want an explicit false", cfg.Storage.S3.PathStyle)
	}
	if err := cfg.Storage.Validate(); err != nil {
		t.Errorf("the configured section should be usable: %v", err)
	}
}

func TestSetConfigValueStorageRejectsBadValues(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "storage.type", "gcs"); err == nil {
		t.Error("an unknown storage type must be rejected")
	} else if !strings.Contains(err.Error(), "storage.type") {
		t.Errorf("error does not name the key: %v", err)
	}
	if cfg.Storage != nil {
		t.Errorf("a rejected value must not create the section: %+v", cfg.Storage)
	}

	if err := setConfigValue(cfg, "storage.s3.path_style", "sometimes"); err == nil {
		t.Error("a non-boolean path_style must be rejected")
	}
	if err := setConfigValue(cfg, "storage.s3.region_name", "us-west-2"); err == nil {
		t.Error("an unknown s3 field must be rejected")
	} else if !strings.Contains(err.Error(), "supported fields") {
		t.Errorf("error does not list the fields: %v", err)
	}
}

func TestUnsetStorage(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	writeConfigFile(t, home, `{"provider":"anthropic","storage":{"type":"s3","s3":{"bucket":"ocr"}}}`)

	if err := runConfigUnset("storage"); err != nil {
		t.Fatalf("runConfigUnset: %v", err)
	}
	cfg, err := loadOrCreateConfig(filepath.Join(home, ".opencodereview", "config.json"))
	if err != nil {
		t.Fatalf("loadOrCreateConfig: %v", err)
	}
	if cfg.Storage != nil {
		t.Errorf("storage section survived unset: %+v", cfg.Storage)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("unset removed an unrelated key: %+v", cfg)
	}
}

func TestStorageSecretsAreMasked(t *testing.T) {
	for _, key := range []string{
		"storage.s3.secret_access_key",
		"storage.s3.SecretAccessKey",
		"storage.s3.session_token",
	} {
		if !shouldMaskConfigValue(key) {
			t.Errorf("%s is a credential and must be masked", key)
		}
	}
	if shouldMaskConfigValue("storage.s3.bucket") {
		t.Error("a bucket name is not a secret")
	}
}

func TestConfigureSessionStorageInstallsBackend(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Cleanup(func() { session.ConfigureStorage(nil) })

	writeConfigFile(t, home, `{
		"storage": {
			"type": "filesystem",
			"path": "`+filepath.ToSlash(filepath.Join(home, "mirror"))+`",
			"prefix": "prod"
		}
	}`)

	configureSessionStorage()
	backend := session.StorageBackend()
	if backend == nil {
		t.Fatal("a filesystem storage section produced no backend")
	}
	if backend.Name() != storage.TypeFilesystem {
		t.Errorf("backend = %q, want %q", backend.Name(), storage.TypeFilesystem)
	}
}

func TestConfigureSessionStorageReportsUnusableSection(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Cleanup(func() { session.ConfigureStorage(nil) })

	// s3 without a bucket: the section exists but cannot be opened.
	writeConfigFile(t, home, `{"storage":{"type":"s3","s3":{"region":"us-west-2"}}}`)

	warning := captureMainStderr(t, configureSessionStorage)
	if !strings.Contains(warning, "object storage disabled") || !strings.Contains(warning, "s3.bucket") {
		t.Errorf("stderr does not explain the disabled store: %q", warning)
	}
	if session.StorageBackend() != nil {
		t.Error("an unusable section must not install a backend")
	}
}

func TestConfigureSessionStorageLeavesLocalInstallsAlone(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	t.Cleanup(func() { session.ConfigureStorage(nil) })

	// No config file at all is the common case and must stay silent.
	warning := captureMainStderr(t, configureSessionStorage)
	if warning != "" {
		t.Errorf("an unconfigured install printed %q", warning)
	}
	if session.StorageBackend() != nil {
		t.Error("an unconfigured install installed a backend")
	}
}

// captureMainStderr returns everything a call writes to stderr.
func captureMainStderr(t *testing.T, run func()) string {
	t.Helper()
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = writer
	defer func() { os.Stderr = original }()

	run()
	if err := writer.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(data)
}

func TestStorageSectionRoundTripsThroughConfigFile(t *testing.T) {
	home := t.TempDir()
	setTestHome(t, home)
	path := filepath.Join(home, ".opencodereview", "config.json")

	cfg, err := loadOrCreateConfig(path)
	if err != nil {
		t.Fatalf("loadOrCreateConfig: %v", err)
	}
	if err := setConfigValue(cfg, "storage.type", "s3"); err != nil {
		t.Fatalf("set storage.type: %v", err)
	}
	if err := setConfigValue(cfg, "storage.s3.bucket", "ocr"); err != nil {
		t.Fatalf("set storage.s3.bucket: %v", err)
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var document struct {
		Storage *storage.FileConfig `json:"storage"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if document.Storage == nil || document.Storage.Type != "s3" || document.Storage.S3.Bucket != "ocr" {
		t.Fatalf("config.json does not carry the storage section: %s", raw)
	}

	// The CLI and the storage layer read the same file: this is the section the
	// mirror resolves at startup.
	loaded, err := storage.LoadConfig(path)
	if err != nil {
		t.Fatalf("storage.LoadConfig: %v", err)
	}
	if loaded.Type != storage.TypeS3 || loaded.S3.Bucket != "ocr" {
		t.Errorf("resolved config = %+v", loaded)
	}
}
