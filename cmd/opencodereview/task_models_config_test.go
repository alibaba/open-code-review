// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetConfigValueTaskModels(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "task_models.plan_task", " cheap-model "); err != nil {
		t.Fatalf("setConfigValue: %v", err)
	}
	if cfg.TaskModels["plan_task"] != "cheap-model" {
		t.Fatalf("TaskModels = %v, want trimmed override", cfg.TaskModels)
	}
}

func TestSetConfigValueTaskModelsRejectsUnknownTask(t *testing.T) {
	cfg := &Config{}
	err := setConfigValue(cfg, "task_models.plan_tasks", "cheap-model")
	if err == nil {
		t.Fatal("expected error for unknown task id")
	}
	if !strings.Contains(err.Error(), `unknown task "plan_tasks"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestSetConfigValueTaskModelsRejectsEmptyModel(t *testing.T) {
	cfg := &Config{}
	if err := setConfigValue(cfg, "task_models.main_task", "  "); err == nil {
		t.Fatal("expected error for empty model")
	}
}

// TestConfigSetTaskModelsRoundTrip covers the persisted path: set writes the
// key to the config file, a second set preserves it, and unset removes it —
// the #1508 unknown-key guarantee does not cover keys the struct knows, so the
// struct round-trip must be pinned.
func TestConfigSetTaskModelsRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	if err := runConfigSet("task_models.plan_task", "cheap-plan"); err != nil {
		t.Fatalf("runConfigSet: %v", err)
	}
	if err := runConfigSet("provider", "anthropic"); err != nil {
		t.Fatalf("runConfigSet provider: %v", err)
	}

	cfgPath := filepath.Join(home, ".opencodereview", "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var persisted struct {
		TaskModels map[string]string `json:"task_models"`
		Provider   string            `json:"provider"`
	}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if persisted.TaskModels["plan_task"] != "cheap-plan" || persisted.Provider != "anthropic" {
		t.Fatalf("persisted = %v / provider %q", persisted.TaskModels, persisted.Provider)
	}

	if err := runConfigUnset("task_models.plan_task"); err != nil {
		t.Fatalf("runConfigUnset: %v", err)
	}
	raw, err = os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("reread config: %v", err)
	}
	var after struct {
		TaskModels map[string]string `json:"task_models"`
	}
	if err := json.Unmarshal(raw, &after); err != nil {
		t.Fatalf("unmarshal after unset: %v", err)
	}
	if _, ok := after.TaskModels["plan_task"]; ok {
		t.Fatalf("task_models still present after unset: %v", after.TaskModels)
	}
}

func TestUnsetTaskModelRejectsUnknownTask(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := runConfigUnset("task_models.typo_task"); err == nil {
		t.Fatal("expected error for unknown task id")
	}
}
