// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"strings"
	"testing"
)

func TestMCPCompactDashboardCopy(t *testing.T) {
	m := dashboardFixture()
	view := m.View().Content
	for _, label := range []string{"MCP servers", "CONFIGURED SERVERS", "MANAGEMENT ACTIONS", "Add server", "Permissions / timeout", "Enter Open"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing navigation: %s", label)
		}
	}
	for _, prose := range []string{"Select a server to manage", "opening this manager", "Configuration status only"} {
		if strings.Contains(view, prose) {
			t.Fatalf("redundant prose: %s", prose)
		}
	}
}

func TestMCPCompletedDiscoveryClearsProgressCopy(t *testing.T) {
	m := appFixture(t)
	m.cancel = func() {}
	done := mcpActionDone{dashboard: m.dashboard, notice: "Checking connection..."}
	done.dashboard.action = "discover"
	m, _ = appMessage(m, done)
	if strings.Contains(m.View().Content, "Checking connection") {
		t.Fatal("completed action still appears busy")
	}
}

func TestMCPCompactSetupKeepsConsent(t *testing.T) {
	m, err := newMCPSetupModel(context.Background(), &Config{}, "docs", false, mcpAddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	view := m.View().Content
	if strings.Count(view, "Connection name") != 1 || strings.Contains(view, "MCP setup") {
		t.Fatal("duplicate page heading", view)
	}
	if strings.Contains(view, "Arrows") || strings.Contains(view, "Space Select") {
		t.Fatal("irrelevant text input keys", view)
	}
	m.screen = mcpSetupConnect
	m.server.Command = "never-start"
	view = m.View().Content
	for _, required := range []string{"never-start", "may have side effects", "nothing is enabled or called", "Save disabled"} {
		if !strings.Contains(view, required) {
			t.Fatalf("consent missing %s", required)
		}
	}
	m.screen = mcpSetupCredentials
	view = m.View().Content
	if !strings.Contains(view, "not tokens") || strings.Contains(view, "Configured:") {
		t.Fatal("credential guidance or empty-state regression", view)
	}
	m.screen = mcpSetupSave
	view = m.View().Content
	if strings.Contains(view, "Legacy setup") || !strings.Contains(view, "Will save disabled") {
		t.Fatal("save summary is not contextual", view)
	}
	m.server.Setup = "never execute"
	if !strings.Contains(m.View().Content, "Legacy setup will be removed, not executed") {
		t.Fatal("legacy warning missing")
	}
}
