package telemetry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Enabled {
		t.Error("expected Enabled=false by default")
	}
	if cfg.ServiceName != "open-code-review" {
		t.Errorf("expected ServiceName=open-code-review, got %s", cfg.ServiceName)
	}
	if cfg.Exporter != "console" {
		t.Errorf("expected Exporter=console, got %s", cfg.Exporter)
	}
	if cfg.OTLPEndpoint != "" {
		t.Errorf("expected empty OTLPEndpoint, got %s", cfg.OTLPEndpoint)
	}
	if cfg.OTLPProtocol != "grpc" {
		t.Errorf("expected OTLPProtocol=grpc, got %s", cfg.OTLPProtocol)
	}
	if cfg.ContentLog {
		t.Error("expected ContentLog=false by default")
	}
}

func TestResolveEnv(t *testing.T) {
	tests := []struct {
		name  string
		envs  map[string]string
		check func(t *testing.T, cfg Config)
	}{
		{
			name: "enable telemetry",
			envs: map[string]string{"OCR_ENABLE_TELEMETRY": "1"},
			check: func(t *testing.T, cfg Config) {
				if !cfg.Enabled {
					t.Error("expected Enabled=true")
				}
			},
		},
		{
			name: "custom service name",
			envs: map[string]string{"OTEL_SERVICE_NAME": "my-service"},
			check: func(t *testing.T, cfg Config) {
				if cfg.ServiceName != "my-service" {
					t.Errorf("expected ServiceName=my-service, got %s", cfg.ServiceName)
				}
			},
		},
		{
			name: "otlp endpoint sets exporter to otlp",
			envs: map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "localhost:4317"},
			check: func(t *testing.T, cfg Config) {
				if cfg.Exporter != "otlp" {
					t.Errorf("expected Exporter=otlp, got %s", cfg.Exporter)
				}
				if cfg.OTLPEndpoint != "localhost:4317" {
					t.Errorf("expected OTLPEndpoint=localhost:4317, got %s", cfg.OTLPEndpoint)
				}
			},
		},
		{
			name: "otlp protocol override",
			envs: map[string]string{"OTEL_EXPORTER_OTLP_PROTOCOL": "http/json"},
			check: func(t *testing.T, cfg Config) {
				if cfg.OTLPProtocol != "http/json" {
					t.Errorf("expected OTLPProtocol=http/json, got %s", cfg.OTLPProtocol)
				}
			},
		},
		{
			name: "content logging enabled",
			envs: map[string]string{"OCR_CONTENT_LOGGING": "1"},
			check: func(t *testing.T, cfg Config) {
				if !cfg.ContentLog {
					t.Error("expected ContentLog=true")
				}
			},
		},
		{
			name: "no env vars leaves defaults",
			envs: map[string]string{},
			check: func(t *testing.T, cfg Config) {
				if cfg.Enabled {
					t.Error("expected Enabled=false")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clear relevant env vars
			envKeys := []string{
				"OCR_ENABLE_TELEMETRY", "OTEL_SERVICE_NAME",
				"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL",
				"OCR_CONTENT_LOGGING",
			}
			for _, k := range envKeys {
				t.Setenv(k, "")
				os.Unsetenv(k)
			}
			for k, v := range tc.envs {
				t.Setenv(k, v)
			}

			cfg := DefaultConfig()
			resolveEnv(&cfg)
			tc.check(t, cfg)
		})
	}
}

func TestLoadFromJSON(t *testing.T) {
	t.Run("nonexistent file returns nil error", func(t *testing.T) {
		cfg := DefaultConfig()
		err := LoadFromJSON(&cfg, "/nonexistent/path/config.json")
		if err != nil {
			t.Errorf("expected nil error for missing file, got %v", err)
		}
	})

	t.Run("malformed json returns nil error", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")
		os.WriteFile(path, []byte("{invalid json"), 0644)

		cfg := DefaultConfig()
		err := LoadFromJSON(&cfg, path)
		if err != nil {
			t.Errorf("expected nil error for malformed JSON, got %v", err)
		}
	})

	t.Run("no telemetry section", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")
		os.WriteFile(path, []byte(`{"other": "value"}`), 0644)

		cfg := DefaultConfig()
		err := LoadFromJSON(&cfg, path)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if cfg.Enabled {
			t.Error("expected Enabled to remain false")
		}
	})

	t.Run("all fields set", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")
		data := `{
			"telemetry": {
				"enabled": true,
				"exporter": "otlp",
				"otlp_endpoint": "collector:4317",
				"content_logging": true
			}
		}`
		os.WriteFile(path, []byte(data), 0644)

		cfg := DefaultConfig()
		err := LoadFromJSON(&cfg, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.Enabled {
			t.Error("expected Enabled=true")
		}
		if cfg.Exporter != "otlp" {
			t.Errorf("expected Exporter=otlp, got %s", cfg.Exporter)
		}
		if cfg.OTLPEndpoint != "collector:4317" {
			t.Errorf("expected OTLPEndpoint=collector:4317, got %s", cfg.OTLPEndpoint)
		}
		if !cfg.ContentLog {
			t.Error("expected ContentLog=true")
		}
	})

	t.Run("otlp_endpoint auto-sets exporter to otlp", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")
		data := `{"telemetry": {"otlp_endpoint": "localhost:4317"}}`
		os.WriteFile(path, []byte(data), 0644)

		cfg := DefaultConfig()
		err := LoadFromJSON(&cfg, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Exporter != "otlp" {
			t.Errorf("expected Exporter=otlp, got %s", cfg.Exporter)
		}
	})

	t.Run("exporter not overridden if already non-default", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")
		data := `{"telemetry": {"exporter": "otlp"}}`
		os.WriteFile(path, []byte(data), 0644)

		cfg := DefaultConfig()
		cfg.Exporter = "custom"
		err := LoadFromJSON(&cfg, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Exporter != "custom" {
			t.Errorf("expected Exporter to remain custom, got %s", cfg.Exporter)
		}
	})
}

func TestResolveConfig(t *testing.T) {
	t.Run("empty path uses only env and defaults", func(t *testing.T) {
		envKeys := []string{
			"OCR_ENABLE_TELEMETRY", "OTEL_SERVICE_NAME",
			"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL",
			"OCR_CONTENT_LOGGING",
		}
		for _, k := range envKeys {
			t.Setenv(k, "")
			os.Unsetenv(k)
		}

		cfg := ResolveConfig("")
		if cfg.Enabled {
			t.Error("expected disabled with no env")
		}
	})

	t.Run("env overrides json", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.json")
		data := `{"telemetry": {"enabled": false}}`
		os.WriteFile(path, []byte(data), 0644)

		t.Setenv("OCR_ENABLE_TELEMETRY", "1")

		cfg := ResolveConfig(path)
		if !cfg.Enabled {
			t.Error("expected env to override json: Enabled should be true")
		}
	})
}

func TestHomeConfigPath(t *testing.T) {
	path := HomeConfigPath()
	if path == "" {
		t.Skip("could not determine home dir")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %s", path)
	}
	if filepath.Base(path) != "config.json" {
		t.Errorf("expected config.json at end, got %s", filepath.Base(path))
	}
}
