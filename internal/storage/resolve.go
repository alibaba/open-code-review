// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Environment variables that override the configuration file, so CI can point a
// run at a bucket without writing credentials to disk.
const (
	EnvType           = "OCR_STORAGE_TYPE"
	EnvPrefix         = "OCR_STORAGE_PREFIX"
	EnvPath           = "OCR_STORAGE_PATH"
	EnvS3Endpoint     = "OCR_S3_ENDPOINT"
	EnvS3Bucket       = "OCR_S3_BUCKET"
	EnvS3Region       = "OCR_S3_REGION"
	EnvS3AccessKeyID  = "OCR_S3_ACCESS_KEY_ID"
	EnvS3SecretKey    = "OCR_S3_SECRET_ACCESS_KEY"
	EnvS3SessionToken = "OCR_S3_SESSION_TOKEN"
	EnvS3PathStyle    = "OCR_S3_PATH_STYLE"
)

// FileConfig is the storage section of ~/.opencodereview/config.json, which is
// why it carries the JSON tags the CLI's own Config struct reuses. Only the keys
// the backend needs are decoded; unknown keys are ignored, as they are for every
// other section of that file.
type FileConfig struct {
	// Type is "local" (the default), "s3" or "filesystem".
	Type string `json:"type,omitempty"`
	// Prefix is prepended to every object key.
	Prefix string `json:"prefix,omitempty"`
	// Path is the root directory of the filesystem backend.
	Path string `json:"path,omitempty"`
	// S3 configures the s3 backend.
	S3 FileS3Config `json:"s3,omitempty"`
}

// FileS3Config is the s3 block of the storage section.
type FileS3Config struct {
	Endpoint        string `json:"endpoint"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	// PathStyle is a pointer so an absent key keeps the default (true) instead
	// of decoding as false. S3-compatible servers need path-style addressing,
	// and a user who sets an endpoint without setting this key means "it is a
	// compatible endpoint".
	PathStyle *bool `json:"path_style"`
}

// Config returns the resolved configuration for this file section.
func (f FileConfig) Config() Config {
	cfg := Config{
		Type:   f.Type,
		Prefix: f.Prefix,
		Path:   f.Path,
		S3: S3Config{
			Endpoint:        f.S3.Endpoint,
			Bucket:          f.S3.Bucket,
			Region:          f.S3.Region,
			AccessKeyID:     f.S3.AccessKeyID,
			SecretAccessKey: f.S3.SecretAccessKey,
			SessionToken:    f.S3.SessionToken,
			PathStyle:       true,
		},
	}
	if f.S3.PathStyle != nil {
		cfg.S3.PathStyle = *f.S3.PathStyle
	}
	return cfg
}

// Validate reports whether this file section describes a usable store.
func (f FileConfig) Validate() error { return f.Config().Validate() }

// ConfigPath returns the default configuration file location,
// $HOME/.opencodereview/config.json.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", "config.json"), nil
}

// LoadConfig reads the storage configuration from path (the default location
// when path is empty) and applies the OCR_* environment overrides on top.
//
// A missing file is not an error: an install that never configured storage has
// no storage section, and that means the local cache is the only store.
func LoadConfig(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		defaultPath, err := ConfigPath()
		if err != nil {
			return Config{}, err
		}
		path = defaultPath
	}

	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
	} else {
		var file struct {
			Storage *FileConfig `json:"storage"`
		}
		if err := json.Unmarshal(data, &file); err != nil {
			return Config{}, fmt.Errorf("parse config %q: %w", path, err)
		}
		if file.Storage != nil {
			cfg = file.Storage.Config()
		}
	}
	return applyEnv(cfg)
}

// applyEnv overlays the non-empty OCR_* variables on cfg.
func applyEnv(cfg Config) (Config, error) {
	for name, target := range map[string]*string{
		EnvType:           &cfg.Type,
		EnvPrefix:         &cfg.Prefix,
		EnvPath:           &cfg.Path,
		EnvS3Endpoint:     &cfg.S3.Endpoint,
		EnvS3Bucket:       &cfg.S3.Bucket,
		EnvS3Region:       &cfg.S3.Region,
		EnvS3AccessKeyID:  &cfg.S3.AccessKeyID,
		EnvS3SecretKey:    &cfg.S3.SecretAccessKey,
		EnvS3SessionToken: &cfg.S3.SessionToken,
	} {
		if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
			*target = value
		}
	}
	if value, ok := os.LookupEnv(EnvS3PathStyle); ok && strings.TrimSpace(value) != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return Config{}, fmt.Errorf("%s=%q is not a boolean", EnvS3PathStyle, value)
		}
		cfg.S3.PathStyle = parsed
	}
	return cfg, nil
}
