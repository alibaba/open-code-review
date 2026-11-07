// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import "testing"

// TestSetConfigValueAcceptsTheGoFieldSpellingOfTheS3Block pins that the s3 block
// answers to both spellings. The routing guard used to match only lowercase
// "storage.s3.", so "storage.S3.bucket" fell through to the unknown-key error
// even though the field parser already unwrapped that form.
func TestSetConfigValueAcceptsTheGoFieldSpellingOfTheS3Block(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "storage.S3.bucket", "ocr"); err != nil {
		t.Fatalf("set storage.S3.bucket: %v", err)
	}
	if cfg.Storage == nil {
		t.Fatal("storage.S3.bucket did not create the storage section")
	}
	if cfg.Storage.S3.Bucket != "ocr" {
		t.Errorf("bucket = %q, want it set through the Go-field spelling", cfg.Storage.S3.Bucket)
	}

	if err := setConfigValue(cfg, "storage.S3.region", "us-west-2"); err != nil {
		t.Fatalf("set storage.S3.region: %v", err)
	}
	if cfg.Storage.S3.Region != "us-west-2" {
		t.Errorf("region = %q, want it set through the Go-field spelling", cfg.Storage.S3.Region)
	}

	// The documented spelling keeps working, and both spellings address the same
	// block rather than two of them.
	if err := setConfigValue(cfg, "storage.s3.endpoint", "https://s3.internal"); err != nil {
		t.Fatalf("set storage.s3.endpoint: %v", err)
	}
	if cfg.Storage.S3.Endpoint != "https://s3.internal" {
		t.Errorf("endpoint = %q", cfg.Storage.S3.Endpoint)
	}
	if err := cfg.Storage.Validate(); err != nil {
		t.Errorf("the section should be usable as configured: %v", err)
	}
}
