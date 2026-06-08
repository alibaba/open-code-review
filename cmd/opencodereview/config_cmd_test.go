package main

import "testing"

func TestSetConfigValueAuthHeaderNormalizesKnownValues(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.auth_header", " bearer "); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}

	if cfg.Llm.AuthHeader != "authorization" {
		t.Errorf("AuthHeader = %q, want %q", cfg.Llm.AuthHeader, "authorization")
	}
}

func TestSetConfigValueAuthHeaderTrimsCustomHeader(t *testing.T) {
	cfg := &Config{}

	if err := setConfigValue(cfg, "llm.auth_header", " X-Custom-Auth "); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}

	if cfg.Llm.AuthHeader != "X-Custom-Auth" {
		t.Errorf("AuthHeader = %q, want %q", cfg.Llm.AuthHeader, "X-Custom-Auth")
	}
}
