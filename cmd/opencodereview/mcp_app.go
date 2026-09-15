// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

type mcpUIContextKey struct{}

// The host adapts transactional command flows to a single terminal owner.
// Only the action goroutine writes output; each prompt transfers a snapshot.
type mcpUIHost struct {
	requests chan mcpPageRequest
	output   bytes.Buffer
}

func (h *mcpUIHost) Write(p []byte) (int, error) {
	const limit = 64 * 1024
	n := len(p)
	if h.output.Len() < limit {
		_, _ = h.output.Write(p[:min(len(p), limit-h.output.Len())])
	}
	return n, nil
}

func mcpHost(ctx context.Context) *mcpUIHost {
	if ctx == nil {
		return nil
	}
	h, _ := ctx.Value(mcpUIContextKey{}).(*mcpUIHost)
	return h
}

type mcpPageRequest struct {
	ctx     context.Context
	model   tea.Model
	preview string
	result  chan tea.Model
}

// Standalone commands retain their reader/writer API. Inside the manager they
// request a child page instead of starting another terminal event loop.
func runMCPModel(ctx context.Context, model tea.Model, in io.Reader, out io.Writer) (tea.Model, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if h := mcpHost(ctx); h != nil {
		request := mcpPageRequest{ctx: ctx, model: model, preview: h.output.String(), result: make(chan tea.Model, 1)}
		h.output.Reset()
		select {
		case h.requests <- request:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case result := <-request.result:
			return result, ctx.Err()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
}

type mcpChildMsg struct {
	generation uint64
	msg        tea.Msg
}

// Existing Bubbles inputs use Batch for cursor updates. Tag each child command
// (including batches) so a late discovery or blink cannot reach a later page.
func mcpChildCommand(generation uint64, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			commands := make([]tea.Cmd, len(batch))
			for i, command := range batch {
				commands[i] = mcpChildCommand(generation, command)
			}
			return tea.BatchMsg(commands)
		}
		return mcpChildMsg{generation, msg}
	}
}

type mcpActionDone struct {
	dashboard mcpDashboardModel
	err       error
	notice    string
}

type mcpAppModel struct {
	dashboard  mcpDashboardModel
	command    *cobra.Command
	ctx        context.Context
	host       *mcpUIHost
	child      *mcpPageRequest
	generation uint64
	busy       bool
	cancel     context.CancelFunc
	scroll     int
}

func (m mcpAppModel) waitPage() tea.Cmd {
	return func() tea.Msg {
		select {
		case request := <-m.host.requests:
			return request
		case <-m.ctx.Done():
			return nil
		}
	}
}

func (m mcpAppModel) Init() tea.Cmd { return m.waitPage() }

func (m mcpAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mcpPageRequest:
		if msg.ctx != nil && msg.ctx.Err() != nil {
			return m, m.waitPage()
		}
		m.generation++
		m.scroll = 0
		m.child = &msg
		init := msg.model.Init()
		updated, resize := msg.model.Update(tea.WindowSizeMsg{Width: m.dashboard.width, Height: max(8, m.dashboard.height-4)})
		m.child.model = updated
		return m, tea.Batch(m.waitPage(), mcpChildCommand(m.generation, init), mcpChildCommand(m.generation, resize))
	case mcpChildMsg:
		if m.child == nil || msg.generation != m.generation {
			return m, nil
		}
		if _, quit := msg.msg.(tea.QuitMsg); quit {
			m.child.result <- m.child.model
			m.child = nil
			m.scroll = 0
			return m, nil
		}
		updated, command := m.child.model.Update(msg.msg)
		m.child.model = updated
		return m, mcpChildCommand(m.generation, command)
	case mcpActionDone:
		previous := m.dashboard
		m.cancel()
		m.cancel = nil
		m.busy = false
		m.child = nil
		m.scroll = 0
		msg.dashboard.width, msg.dashboard.height = m.dashboard.width, m.dashboard.height
		m.dashboard = msg.dashboard
		m.dashboard.action = ""
		m.dashboard.notice = m.dashboard.safe(strings.TrimSpace(msg.notice))
		if msg.dashboard.action == "discover" && msg.err == nil {
			// The dashboard already shows the completed check; discard progress copy.
			m.dashboard.notice = ""
		}
		if msg.err != nil {
			m.dashboard.notice = "Action failed. Check settings and retry."
			if errors.Is(msg.err, errConfigConflict) || errors.Is(msg.err, errConfigBusy) {
				m.dashboard.notice = "Configuration changed or is busy. Nothing saved; reopen settings and retry."
			}
		}
		if _, ok := m.dashboard.cfg.MCPServers[m.dashboard.server]; m.dashboard.server != "" && !ok {
			m.dashboard.server, m.dashboard.page, m.dashboard.cursor = "", "", 0
		}
		if previous.page == "" {
			for i, item := range m.dashboard.items() {
				if item.action == previous.action {
					m.dashboard.cursor = i
				}
				if (previous.action == "add" || previous.action == "import") && item.action == "server" {
					if _, existed := previous.cfg.MCPServers[item.identity]; !existed {
						m.dashboard.cursor = i
						break
					}
				}
			}
		}
		m.dashboard.cursor = min(m.dashboard.cursor, len(m.dashboard.items())-1)
		return m, nil
	case tea.WindowSizeMsg:
		m.dashboard.width, m.dashboard.height = msg.Width, msg.Height
		if m.child != nil {
			msg.Height = max(8, msg.Height-4)
			updated, command := m.child.model.Update(msg)
			m.child.model = updated
			return m, mcpChildCommand(m.generation, command)
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "ctrl+d" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if m.child != nil {
			switch msg.String() {
			case "pgup":
				m.scroll = max(0, m.scroll-max(1, m.dashboard.height-6))
				return m, nil
			case "pgdown":
				m.scroll = min(m.maxScroll(), m.scroll+max(1, m.dashboard.height-6))
				return m, nil
			}
		} else if m.busy && msg.String() == "esc" {
			m.cancel()
		}
	}
	if m.child != nil {
		updated, command := m.child.model.Update(msg)
		m.child.model = updated
		return m, mcpChildCommand(m.generation, command)
	}
	if m.busy {
		return m, nil
	}
	updated, command := m.dashboard.Update(msg)
	m.dashboard = updated.(mcpDashboardModel)
	if m.dashboard.action == "" || m.dashboard.action == "quit" {
		return m, command
	}
	m.busy = true
	m.dashboard.notice = ""
	snapshot := m.dashboard
	snapshot.checks = maps.Clone(m.dashboard.checks)
	ctx, cancel := context.WithCancel(context.WithValue(m.ctx, mcpUIContextKey{}, m.host))
	m.cancel = cancel
	// Reuse the existing configuration transactions without letting them print
	// around the active renderer or mutate the UI's navigation state.
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.SetIn(m.command.InOrStdin())
	cmd.SetOut(m.host)
	cmd.SetErr(m.host)
	return m, func() tea.Msg {
		err := snapshot.perform(cmd)
		cfg, loadErr := loadReadOnlyMCPConfig()
		if loadErr != nil {
			err = loadErr
		} else {
			snapshot.cfg = cfg
		}
		notice := m.host.output.String()
		m.host.output.Reset()
		return mcpActionDone{snapshot, err, notice}
	}
}

func (m mcpAppModel) childLines() []string {
	content := m.child.model.View().Content
	if m.child.preview != "" {
		content = m.child.preview + "\n" + content
	}
	return strings.Split(ansi.Hardwrap(content, max(20, m.dashboard.width), true), "\n")
}

func (m mcpAppModel) maxScroll() int {
	return max(0, len(m.childLines())-max(1, m.dashboard.height-4))
}

func (m mcpAppModel) View() tea.View {
	var view tea.View
	if m.child != nil {
		lines := m.childLines()
		start := min(m.scroll, m.maxScroll())
		end := min(len(lines), start+max(1, m.dashboard.height-4))
		view = tea.NewView(strings.Join(lines[start:end], "\n"))
		if m.maxScroll() > 0 {
			view.Content += "\n" + tuiHelpStyle.Render("PgUp/PgDn Scroll")
		}
	} else if m.busy {
		view = tea.NewView("Working...\n\nEsc Cancel · Ctrl-C Quit")
	} else {
		view = m.dashboard.View()
	}
	view.AltScreen = true
	return view
}
