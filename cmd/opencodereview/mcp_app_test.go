// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func appFixture(t *testing.T) mcpAppModel {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd, _, _ := newMCPTestCommand("")
	cmd.SetContext(ctx)
	dashboard := dashboardFixture()
	dashboard.width = 80
	return mcpAppModel{dashboard: dashboard, command: cmd, ctx: ctx, host: &mcpUIHost{requests: make(chan mcpPageRequest)}}
}

func appMessage(m mcpAppModel, msg tea.Msg) (mcpAppModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(mcpAppModel), cmd
}

func TestMCPAppPageLifecycleAndFocus(t *testing.T) {
	m := appFixture(t)
	m.dashboard.cursor = 1 // Add server, after one configured server.
	m.dashboard.action = "add"
	m.busy = true
	m.cancel = func() {}
	request := mcpPageRequest{model: newMCPManagementTextModel("Connection name", ""), result: make(chan tea.Model, 1)}
	m, _ = appMessage(m, request)
	view := m.View()
	if !view.AltScreen || !strings.Contains(view.Content, "Connection name") || strings.Contains(view.Content, "CONFIGURED SERVERS") {
		t.Fatal("Child page must replace, not append to, the dashboard", view.Content)
	}
	m, command := appMessage(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	msg := command()
	if _, exits := msg.(tea.QuitMsg); exits {
		t.Fatal("Child cancellation must not quit the terminal owner")
	}
	m, _ = appMessage(m, msg)
	final := (<-request.result).(mcpManagementTextModel)
	if !final.cancelled || final.submitted || m.child != nil {
		t.Fatal("Cancellation became a submission")
	}
	m, _ = appMessage(m, mcpActionDone{dashboard: m.dashboard, notice: "Cancelled. No changes saved."})
	if m.busy || m.dashboard.cursor != 1 || !m.View().AltScreen || strings.Contains(m.View().Content, "Connection name") {
		t.Fatal("Parent focus or screen lifecycle changed")
	}
	if !strings.Contains(m.View().Content, "CONFIGURED SERVERS") || !strings.Contains(m.View().Content, "MANAGEMENT ACTIONS") {
		t.Fatal("Servers and management actions are not separate")
	}
	m, _ = appMessage(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.dashboard.cursor != 0 {
		t.Fatal("Tab did not switch to servers")
	}
	m, _ = appMessage(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.dashboard.cursor != 1 {
		t.Fatal("Tab did not switch to management actions")
	}
}

func TestMCPAppStaleCommandsAndScrolling(t *testing.T) {
	m := appFixture(t)
	request := mcpPageRequest{model: newMCPManagementConfirmModel("Connect?"), preview: strings.Repeat("Preview line\n", 40), result: make(chan tea.Model, 1)}
	m, _ = appMessage(m, request)
	oldGeneration := m.generation
	m, _ = appMessage(m, request)
	m, _ = appMessage(m, mcpChildMsg{oldGeneration, tea.QuitMsg{}})
	if m.child == nil {
		t.Fatal("Old child command closed a new page")
	}
	m, _ = appMessage(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.scroll == 0 {
		t.Fatal("Long connection preview cannot be scrolled")
	}
	m, _ = appMessage(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if !strings.Contains(m.View().Content, "Connect?") {
		t.Fatal("Confirmation was lost below the viewport")
	}
	m, _ = appMessage(m, tea.WindowSizeMsg{Width: 60, Height: 16})
	if len(strings.Split(m.View().Content, "\n")) > 16 {
		t.Fatal("Child view overflows screen")
	}
	m, _ = appMessage(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.scroll >= m.maxScroll() {
		t.Fatal("Page up did not move")
	}
	if mcpChildCommand(1, nil) != nil {
		t.Fatal("nil command became active")
	}
	batch := mcpChildCommand(7, tea.Batch(tea.Quit, tea.Quit))().(tea.BatchMsg)
	for _, command := range batch {
		if command().(mcpChildMsg).generation != 7 {
			t.Fatal("Batch command lost page identity")
		}
	}
}

func TestMCPHostedPromptAndCancellation(t *testing.T) {
	for _, cancelBeforeRequest := range []bool{true, false} {
		t.Run(map[bool]string{true: "queued", false: "displayed"}[cancelBeforeRequest], func(t *testing.T) {
			h := &mcpUIHost{requests: make(chan mcpPageRequest)}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), mcpUIContextKey{}, h))
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				_, err := runMCPModel(ctx, newMCPManagementConfirmModel("Confirm?"), nil, io.Discard)
				finished <- err
			}()
			if !cancelBeforeRequest {
				<-h.requests
			}
			cancel()
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("Cancelled prompt remained blocked")
			}
		})
	}
	m := appFixture(t)
	ctx := context.WithValue(m.ctx, mcpUIContextKey{}, m.host)
	if !(&mcpPrompter{ctx: ctx}).usesBubbleTea() || mcpHost(nil) != nil {
		t.Fatal("Host routing was not recognized")
	}
	_, _ = m.host.Write([]byte(strings.Repeat("x", 100000)))
	_, _ = m.host.Write([]byte("overflow"))
	if m.host.output.Len() != 64*1024 {
		t.Fatal("Unbounded command output")
	}
	finished := make(chan error, 1)
	go func() {
		value, err := runMCPManagementTextPrompt(ctx, "Name", "", nil, io.Discard)
		if value != "docs" && err == nil {
			err = errors.New("lost submitted value")
		}
		finished <- err
	}()
	request := <-m.host.requests
	if len(request.preview) != 64*1024 {
		t.Fatal("Lost transaction preview")
	}
	request.result <- mcpManagementTextModel{submitted: true, value: "docs"}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestMCPAppReusesTransactionsAndCancellation(t *testing.T) {
	m := appFixture(t)
	setupMCPTestHome(t, m.dashboard.cfg)
	setMCPTestInteractive(t, true)
	m.dashboard.page, m.dashboard.server, m.dashboard.tool = "tool", "docs", "search"
	m, action := appMessage(m, tea.KeyPressMsg{Code: tea.KeyEnter}) // Offline revoke.
	if !m.busy || !strings.Contains(m.View().Content, "Working") {
		t.Fatal("Action did not enter busy state")
	}
	done := action().(mcpActionDone)
	m, _ = appMessage(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = appMessage(m, done)
	if m.dashboard.width != 100 || m.dashboard.height != 40 || len(m.dashboard.cfg.MCPServers["docs"].Tools) != 0 {
		t.Fatal("Transaction or resize was lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.busy, m.cancel = true, cancel
	m, _ = appMessage(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if ctx.Err() == nil {
		t.Fatal("Busy escape did not cancel")
	}
	_, quit := appMessage(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl-C did not exit")
	}
}

func TestMCPAppPageQueueAndLateCancellation(t *testing.T) {
	m := appFixture(t)
	ctx, cancel := context.WithCancel(m.ctx)
	m.ctx = ctx
	defer cancel()
	request := mcpPageRequest{ctx: ctx, model: newMCPManagementConfirmModel("Connect?"), result: make(chan tea.Model, 1)}
	ready := make(chan tea.Msg, 1)
	go func() { ready <- m.Init()() }()
	m.host.requests <- request
	m, _ = appMessage(m, <-ready)
	m, _ = appMessage(m, mcpChildMsg{m.generation, tea.KeyPressMsg{Code: tea.KeyDown}})
	if m.child.model.(mcpManagementConfirmModel).selected != 1 {
		t.Fatal("Child message was not routed")
	}
	cancel()
	m.child = nil
	m, waiting := appMessage(m, request)
	if m.child != nil || waiting() != nil {
		t.Fatal("Cancelled request reopened a child page")
	}
	m, _ = appMessage(m, mcpChildMsg{m.generation, tea.QuitMsg{}})
	// Removal and an error must return to a valid navigation page, while never
	// showing server-controlled error strings or a stale server selection.
	m.cancel = func() {}
	m.dashboard.server, m.dashboard.page = "docs", "server"
	done := mcpActionDone{dashboard: m.dashboard, err: errors.New("SECRET_SENTINEL")}
	done.dashboard.cfg = &Config{}
	m, _ = appMessage(m, done)
	if m.dashboard.server != "" || strings.Contains(m.View().Content, "SECRET_SENTINEL") || !strings.Contains(m.View().Content, "Action failed") {
		t.Fatal("Failed action leaked details or left a stale page")
	}
	_, quit := appMessage(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatal("Navigation quit was swallowed")
	}
}

func TestMCPAppNewServerFocus(t *testing.T) {
	m := appFixture(t)
	m.dashboard.action = "import"
	m.dashboard.cursor = 2
	m.cancel = func() {}
	done := mcpActionDone{dashboard: m.dashboard}
	done.dashboard.cfg = &Config{MCPServers: map[string]MCPServerConfig{
		"docs": m.dashboard.cfg.MCPServers["docs"], "new": {Command: "never-start"},
	}}
	m, _ = appMessage(m, done)
	if item := m.dashboard.items()[m.dashboard.cursor]; item.identity != "new" || item.action != "server" {
		t.Fatal("Successful import did not focus the newly added server")
	}
}
