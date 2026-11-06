// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

type tuiScreen int

const (
	tuiHome tuiScreen = iota
	tuiLog
	tuiPreview
	tuiResults
	tuiFinding
	tuiSessions
	tuiHelp
	tuiSettings
)

type tuiTickMsg time.Time
type tuiReviewDoneMsg struct {
	id     int
	result jsonOutput
	err    error
}
type tuiPreviewDoneMsg struct {
	id      int
	preview model.Preview
	err     error
}
type tuiSessionsMsg struct {
	id       int
	sessions []session.Summary
	err      error
}
type tuiSessionMsg struct {
	id       int
	summary  *session.Summary
	comments []model.LlmComment
	resume   bool
	err      error
}
type tuiConfigDoneMsg struct {
	changed bool
	err     error
}

type tuiLoginDoneMsg struct {
	planEnabled bool
	err         error
}

type tuiPreferencesSavedMsg struct {
	preferences tuiPreferences
	err         error
}

// Progress is a bounded tail, sampled by the UI rather than queued one message
// per log write. A chatty review never blocks the child or floods the event loop.
type tuiProgressTail struct {
	mu    sync.Mutex
	lines []string
}

func (p *tuiProgressTail) append(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lines = append(p.lines, ansi.Truncate(sanitizeTerminal(line), 2000, "..."))
	if len(p.lines) > 250 {
		copy(p.lines, p.lines[len(p.lines)-250:])
		p.lines = p.lines[:250]
	}
}

func (p *tuiProgressTail) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.lines...)
}

type reviewTUIModel struct {
	ctx               context.Context
	opts              reviewOptions
	services          tuiServices
	colored           bool
	width             int
	height            int
	input             textinput.Model
	viewport          viewport.Model
	screen            tuiScreen
	notice            string
	isError           bool
	busy              bool
	job               int
	jobName           string
	started           time.Time
	cancel            context.CancelFunc
	stopping          bool
	quit              bool
	progress          *tuiProgressTail
	result            jsonOutput
	resultError       string
	preview           model.Preview
	previewError      string
	sessions          []session.Summary
	selected          int
	session           *session.Summary
	preferences       tuiPreferences
	settingsIndex     int
	settingsReturn    tuiScreen
	savingPreferences bool
	menuSelected      int
	menuDismissed     bool
	history           []string
	historyIndex      int
	historyDraft      string
}

func newReviewTUI(ctx context.Context, opts reviewOptions, services tuiServices, colored bool) reviewTUIModel {
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = "Type a command or describe your changes"
	in.CharLimit = 8192
	in.SetWidth(72)
	in.ShowSuggestions = false
	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#74CA91"))
	styles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#DCE8DF"))
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#82988A"))
	styles.Cursor.Color = lipgloss.Color("#74CA91")
	in.SetStyles(styles)
	in.Focus()
	view := viewport.New(viewport.WithWidth(76), viewport.WithHeight(24))
	view.SoftWrap = true
	view.KeyMap = viewport.KeyMap{
		PageUp:   key.NewBinding(key.WithKeys("pgup")),
		PageDown: key.NewBinding(key.WithKeys("pgdown")),
	}
	m := reviewTUIModel{ctx: ctx, opts: opts, services: services, colored: colored, width: 80, height: 32, input: in, viewport: view, preferences: tuiPreferences{Language: "en"}}
	m.refresh(false)
	return m
}

func (m reviewTUIModel) Init() tea.Cmd { return textinput.Blink }

func tuiTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg { return tuiTickMsg(t) })
}

func (m reviewTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.input.SetWidth(max(1, m.bodyWidth()-3))
		m.viewport.SetWidth(m.bodyWidth())
		m.refresh(false)
		return m, nil
	case tuiTickMsg:
		if m.busy {
			m.refresh(m.screen == tuiLog && m.viewport.AtBottom())
			return m, tuiTick()
		}
		return m, nil
	case tuiReviewDoneMsg:
		if msg.id != m.job {
			return m, nil
		}
		m.finishJob(msg.err)
		m.result, m.session, m.selected = msg.result, nil, 0
		m.resultError = ""
		if msg.err != nil {
			m.resultError = sanitizeTerminal(msg.err.Error())
			// The result pane owns the full diagnostic; do not repeat it at the prompt.
			m.notice = ""
		}
		m.screen = tuiResults
		m.viewport.GotoTop()
		m.refresh(false)
		if m.quit {
			return m, tea.Quit
		}
		return m, nil
	case tuiPreviewDoneMsg:
		if msg.id != m.job {
			return m, nil
		}
		m.finishJob(msg.err)
		m.preview, m.screen = msg.preview, tuiPreview
		m.previewError = ""
		if msg.err != nil {
			m.previewError = sanitizeTerminal(msg.err.Error())
			m.notice = ""
		}
		m.viewport.GotoTop()
		m.refresh(false)
		if m.quit {
			return m, tea.Quit
		}
		return m, nil
	case tuiSessionsMsg:
		if msg.id != m.job {
			return m, nil
		}
		m.finishJob(msg.err)
		m.sessions, m.selected, m.screen = msg.sessions, 0, tuiSessions
		m.viewport.GotoTop()
		m.refresh(false)
		if m.quit {
			return m, tea.Quit
		}
		return m, nil
	case tuiSessionMsg:
		if msg.id != m.job {
			return m, nil
		}
		m.finishJob(msg.err)
		if m.quit {
			return m, tea.Quit
		}
		if msg.err != nil {
			m.refresh(false)
			return m, nil
		}
		if msg.resume {
			if err := m.prepareResume(msg.summary); err != nil {
				m.notice, m.isError = err.Error(), true
				m.refresh(false)
				return m, nil
			}
			m.notice = "Session scope restored. /review to resume with the current model."
			m.screen = tuiHome
		} else {
			m.session = msg.summary
			m.result = jsonOutput{Comments: msg.comments, Manifest: msg.summary.RunManifest, SessionID: msg.summary.SessionID, Message: "Saved session"}
			m.resultError = ""
			m.selected, m.screen = 0, tuiResults
		}
		m.viewport.GotoTop()
		m.refresh(false)
		return m, nil
	case tuiConfigDoneMsg:
		m.finishJob(msg.err)
		if msg.err == nil && msg.changed {
			m.opts.provider, m.opts.model = "", ""
			m.notice = "Configuration updated. The next review uses your configured model."
		} else if msg.err == nil {
			m.notice = "Configuration unchanged. Current selection kept."
		}
		m.refresh(false)
		return m, textinput.Blink
	case tuiLoginDoneMsg:
		m.finishJob(msg.err)
		if msg.err != nil {
			m.notice = m.tr("ChatGPT sign-in did not complete: %s", sanitizeTerminal(msg.err.Error()))
		} else if msg.planEnabled {
			m.notice = m.tr("ChatGPT sign-in completed. Use /provider to configure ChatGPT and its model.")
		} else {
			m.notice = m.tr("Signed in; ChatGPT plan usage is disabled. Run ocr auth login --provider chatgpt --enable-plan to grant permission.")
		}
		m.refresh(false)
		return m, textinput.Blink
	case tuiPreferencesSavedMsg:
		m.savingPreferences = false
		if msg.err != nil {
			m.notice, m.isError = m.tr("Could not save settings: %s", sanitizeTerminal(msg.err.Error())), true
		} else {
			m.preferences = msg.preferences
			m.notice, m.isError = m.tr("Settings saved."), false
		}
		m.refresh(false)
		return m, nil
	case tea.KeyPressMsg:
		if m.screen == tuiSettings && msg.String() != "ctrl+c" {
			return m.updateSettings(msg)
		}
		if choices := m.commandChoices(); len(choices) > 0 {
			switch msg.String() {
			case "up", "down":
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				m.menuSelected = (m.menuSelected + delta + len(choices)) % len(choices)
				m.refresh(false)
				return m, nil
			case "tab", "enter":
				choice := choices[min(m.menuSelected, len(choices)-1)]
				m.menuDismissed = true
				m.input.SetValue(choice.name)
				m.input.CursorEnd()
				if msg.String() == "enter" && choice.immediate {
					m.input.SetValue("")
					m.rememberCommand(choice.name)
					return m.command(choice.name)
				}
				if !choice.immediate {
					m.input.SetValue(choice.name + " ")
					m.input.CursorEnd()
				}
				m.refresh(false)
				return m, nil
			case "esc":
				m.menuDismissed = true
				m.refresh(false)
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+c":
			if m.busy {
				m.quit = true
				m.stopJob()
				return m, nil
			}
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.refresh(false)
				return m, nil
			}
			return m, tea.Quit
		case "esc":
			if m.busy {
				m.stopJob()
				return m, nil
			}
			if m.input.Value() != "" {
				m.input.SetValue("")
			} else if m.screen == tuiFinding {
				m.screen = tuiResults
			} else {
				m.screen = tuiHome
			}
			m.notice, m.isError = "", false
			m.refresh(false)
			return m, nil
		case "enter":
			line := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			m.menuDismissed = false
			if line == "" {
				if m.screen == tuiResults && len(m.result.Comments) > 0 {
					m.screen = tuiFinding
					m.viewport.GotoTop()
					m.refresh(false)
					return m, nil
				}
				if m.screen == tuiSessions && len(m.sessions) > 0 && !m.busy {
					return m.loadSession(m.sessions[m.selected].SessionID, false)
				}
				return m, nil
			}
			m.rememberCommand(line)
			return m.command(line)
		case "up", "down":
			if m.input.Value() == "" {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				count := 0
				if m.screen == tuiResults {
					count = len(m.result.Comments)
				}
				if m.screen == tuiSessions {
					count = len(m.sessions)
				}
				if count > 0 {
					m.selected = max(0, min(count-1, m.selected+delta))
				} else {
					m.moveHistory(delta)
				}
				m.refresh(false)
				if count > 0 {
					m.ensureSelectionVisible()
				}
				return m, nil
			}
			if len(m.history) > 0 {
				delta := 1
				if msg.String() == "up" {
					delta = -1
				}
				m.moveHistory(delta)
				m.refresh(false)
				return m, nil
			}
		case "pgup", "pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.menuSelected, m.menuDismissed = 0, false
		m.historyIndex = len(m.history)
	}
	m.refresh(false)
	return m, cmd
}

func (m *reviewTUIModel) finishJob(err error) {
	if m.cancel != nil {
		m.cancel()
	}
	wasStopping := m.stopping
	m.busy, m.stopping, m.cancel = false, false, nil
	m.notice, m.isError = "", false
	if err != nil {
		m.notice, m.isError = sanitizeTerminal(err.Error()), true
	}
	if wasStopping && err == nil {
		m.notice = "The cancelled task has finished. Check the result for completed work."
	}
}

func (m *reviewTUIModel) stopJob() {
	if m.cancel != nil {
		m.cancel()
	}
	m.stopping = true
	m.notice = "Stopping... waiting for the review to save results."
}

func (m *reviewTUIModel) beginJob(name string) (context.Context, int) {
	m.job++
	m.busy, m.stopping, m.notice, m.isError = true, false, "", false
	m.jobName, m.started = name, time.Now()
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	return ctx, m.job
}

func (m reviewTUIModel) work(run func() tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if m.services.workers != nil {
			if !m.services.workers.start() {
				return nil
			}
			defer m.services.workers.wg.Done()
		}
		return run()
	}
}

func (m reviewTUIModel) startReview(preview bool) (tea.Model, tea.Cmd) {
	ctx, id := m.beginJob("Reviewing")
	opts := m.opts
	m.progress = &tuiProgressTail{}
	m.screen = tuiLog
	m.viewport.GotoTop()
	var run tea.Cmd
	if preview {
		m.jobName = "Inspecting changes"
		opts.resume = ""
		run = m.work(func() tea.Msg {
			result, err := m.services.preview(ctx, opts)
			return tuiPreviewDoneMsg{id, result, err}
		})
	} else {
		tail := m.progress
		run = m.work(func() tea.Msg {
			result, err := m.services.review(ctx, opts, tail.append)
			return tuiReviewDoneMsg{id, result, err}
		})
	}
	m.refresh(false)
	return m, tea.Batch(run, tuiTick())
}

func (m reviewTUIModel) loadSession(id string, resume bool) (tea.Model, tea.Cmd) {
	_, job := m.beginJob("Loading session")
	repo := m.opts.repoDir
	run := m.work(func() tea.Msg {
		summary, comments, err := m.services.load(repo, id)
		return tuiSessionMsg{job, summary, comments, resume, err}
	})
	return m, tea.Batch(run, tuiTick())
}

func (m *reviewTUIModel) prepareResume(s *session.Summary) error {
	if s.RunManifest == nil || s.RunManifest.Operation != session.OperationReview {
		return fmt.Errorf("this session cannot resume a diff review")
	}
	if s.ReviewMode != session.ReviewModeRange && s.ReviewMode != session.ReviewModeCommit {
		return fmt.Errorf("workspace sessions can be viewed; resume requires a branch range or commit")
	}
	m.opts.from, m.opts.to, m.opts.commit = s.DiffFrom, s.DiffTo, s.DiffCommit
	m.opts.resume = s.SessionID
	return nil
}

func (m reviewTUIModel) command(line string) (tea.Model, tea.Cmd) {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	if name == "/help" || name == "?" {
		m.screen = tuiHelp
		m.viewport.GotoTop()
		m.refresh(false)
		return m, nil
	}
	if name == "/logs" {
		m.screen = tuiLog
		m.refresh(true)
		return m, nil
	}
	if name == "/quit" {
		if m.busy {
			m.quit = true
			m.stopJob()
			return m, nil
		}
		return m, tea.Quit
	}
	if name == "/cancel" && m.busy {
		m.stopJob()
		return m, nil
	}
	if m.busy {
		m.notice = "A task is running. Esc cancels it; /logs shows its progress."
		return m, nil
	}
	m.notice, m.isError = "", false
	switch name {
	case "/review", "/preview":
		if arg != "" {
			m.notice, m.isError = "Use /review or /preview without arguments.", true
			break
		}
		return m.startReview(name == "/preview")
	case "/scope":
		parts := strings.Fields(arg)
		opts := m.opts
		opts.from, opts.to, opts.commit, opts.resume = "", "", "", ""
		switch {
		case len(parts) == 1 && parts[0] == "workspace":
		case len(parts) == 2 && parts[0] == "commit":
			opts.commit = parts[1]
		case len(parts) == 3 && parts[0] == "range":
			opts.from, opts.to = parts[1], parts[2]
		default:
			m.notice, m.isError = "Use /scope workspace, /scope commit REF, or /scope range BASE HEAD.", true
			return m, nil
		}
		m.opts, m.screen = opts, tuiHome
		m.notice = "Scope updated. /preview lists the files."
	case "/background":
		m.opts.background, m.opts.backgroundFile = arg, ""
		m.notice = "Review background updated."
	case "/login":
		if arg != "" {
			m.notice, m.isError = m.tr("Use /login to sign in with ChatGPT."), true
			break
		}
		m.busy, m.jobName, m.started = true, "Signing in with ChatGPT", time.Now()
		return m, m.services.login()
	case "/model", "/provider":
		if arg != "" {
			if name == "/model" {
				m.opts.model = arg
			} else {
				m.opts.provider = arg
				m.opts.model = ""
			}
			m.notice = "Selection applies to the next review."
		} else {
			m.busy, m.jobName = true, "Configuring model"
			m.started = time.Now()
			return m, m.services.config(strings.TrimPrefix(name, "/"))
		}
	case "/effort":
		if arg != "low" && arg != "medium" && arg != "high" {
			m.notice, m.isError = "Use /effort low, medium, or high.", true
			break
		}
		m.opts.effort = arg
		m.notice = "Review effort updated."
	case "/budget":
		value, err := strconv.Atoi(arg)
		if err != nil || value < 0 {
			m.notice, m.isError = "Use /budget TOKENS (0 means unlimited).", true
			break
		}
		m.opts.maxTokensBudget = value
		m.notice = "Token budget updated."
	case "/sessions":
		_, id := m.beginJob("Loading sessions")
		repo := m.opts.repoDir
		run := m.work(func() tea.Msg {
			sessions, err := m.services.sessions(repo)
			return tuiSessionsMsg{id, sessions, err}
		})
		return m, tea.Batch(run, tuiTick())
	case "/resume":
		if arg == "" && m.session != nil {
			arg = m.session.SessionID
		}
		if arg == "" {
			m.notice, m.isError = "Use /resume SESSION_ID, or open a saved session first.", true
			break
		}
		return m.loadSession(arg, true)
	case "/results":
		m.screen = tuiResults
		m.selected = max(0, min(m.selected, len(m.result.Comments)-1))
		m.viewport.GotoTop()
		m.refresh(false)
		m.ensureSelectionVisible()
		return m, nil
	case "/settings", "/setting", "/config":
		m.settingsReturn, m.screen = m.screen, tuiSettings
		for i, id := range tuiLanguageIDs {
			if id == m.preferences.Language {
				m.settingsIndex = i
			}
		}
		m.viewport.GotoTop()
	default:
		if strings.HasPrefix(line, "/") {
			m.notice, m.isError = "Unknown command. /help lists available commands.", true
		} else {
			m.opts.background, m.opts.backgroundFile = line, ""
			m.notice = "Background saved. /preview to inspect changes, /review to start."
		}
	}
	m.refresh(false)
	return m, nil
}

func (m reviewTUIModel) tint(s, color string) string {
	if !m.colored {
		return s
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(s)
}

func (m reviewTUIModel) tr(key string, args ...any) string {
	return tuiText(m.preferences.Language, key, args...)
}
