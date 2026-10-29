// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/alibaba/open-code-review/internal/chatgptauth"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

func newTUICmd() *cobra.Command {
	opts := reviewOptions{outputFormat: "json", audience: "human"}
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive review workspace",
		Long: `Review code in an interactive terminal workspace.

Use /scope to select changes, /preview to inspect the files, and /review to
start. Type a description of your changes to set the review background.
Use /model or /provider for configuration, /sessions for history, and /help
for all commands. /settings changes the interface language and remembers it.
Use /login to sign in with ChatGPT, then /provider to configure it.
Esc cancels a running review and waits for saved results.

Requires an interactive terminal. Credential commands used during a review
must work without prompting on stdin; configure credentials beforehand with
'ocr config provider' or run 'ocr review' directly for interactive helpers.`,
		Args: cobra.NoArgs,
		Example: `  ocr tui
  ocr tui --from main --to HEAD
  ocr tui --commit abc123 --background "Fix login retries"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateReviewOptions(&opts); err != nil {
				return err
			}
			if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) || strings.EqualFold(os.Getenv("TERM"), "dumb") {
				return fmt.Errorf("ocr tui requires an interactive terminal; use 'ocr review' for scripts and pipes")
			}
			repoDir, err := resolveRepoDir(opts.repoDir)
			if err != nil {
				return err
			}
			opts.repoDir = repoDir
			return runReviewTUI(cmd.Context(), opts)
		},
	}
	addRepoFlag(cmd, &opts.repoDir)
	addDiffFlags(cmd, &opts.from, &opts.to, &opts.commit)
	addBackgroundFlags(cmd, &opts.background, &opts.backgroundFile)
	addProviderFlag(cmd, &opts.provider)
	addModelFlag(cmd, &opts.model)
	addRuleFlag(cmd, &opts.rulePath)
	addToolsFlag(cmd, &opts.toolConfigPath)
	addExcludeFlag(cmd, &opts.excludes)
	addConcurrencyFlags(cmd, &opts.concurrency, &opts.concurrentTaskTimeout, &opts.maxTools, &opts.maxGitProcs, &opts.maxTokens, &opts.maxTokensBudget)
	cmd.Flags().StringVar(&opts.effort, "effort", "", "review effort: low, medium, or high")
	cmd.Flags().BoolVar(&opts.noFilter, "no-filter", false, "keep findings without LLM post-filtering")
	cmd.Flags().StringVar(&opts.resume, "resume", "", "prepare a review with an existing session checkpoint")
	return cmd
}

// tuiServices keeps terminal state independent of I/O. Reviews run serially in
// a child process, so their global output routing cannot corrupt the renderer.
type tuiServices struct {
	review          func(context.Context, reviewOptions, func(string)) (jsonOutput, error)
	preview         func(context.Context, reviewOptions) (model.Preview, error)
	sessions        func(string) ([]session.Summary, error)
	load            func(string, string) (*session.Summary, []model.LlmComment, error)
	config          func(string) tea.Cmd
	login           func() tea.Cmd
	workers         *tuiWorkers
	savePreferences func(tuiPreferences) error
}

// Closing the worker set prevents a late Bubble Tea command from starting a
// process after Program.Run has returned and shutdown has begun.
type tuiWorkers struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

func (w *tuiWorkers) start() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	w.wg.Add(1)
	return true
}

func (w *tuiWorkers) closeAndWait() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	w.wg.Wait()
}

func runReviewTUI(parent context.Context, opts reviewOptions) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate OCR executable: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	workers := &tuiWorkers{}
	defer func() {
		cancel()
		workers.closeAndWait()
	}()
	services := tuiServices{
		review: runTUIReview, preview: runTUIPreview,
		savePreferences: saveTUIPreferences,
		sessions:        session.ListSessions,
		load: func(repo, id string) (*session.Summary, []model.LlmComment, error) {
			summary, err := session.LoadSummary(repo, id)
			if err != nil {
				return nil, nil, err
			}
			comments, err := session.LoadComments(repo, id)
			return summary, comments, err
		},
		config: func(kind string) tea.Cmd {
			path, err := defaultConfigPath()
			if err != nil {
				return func() tea.Msg { return tuiConfigDoneMsg{err: err} }
			}
			before, err := LoadAppConfig(path)
			if err != nil {
				return func() tea.Msg { return tuiConfigDoneMsg{err: err} }
			}
			cmd := exec.CommandContext(ctx, executable, "config", kind)
			cmd.Dir = opts.repoDir
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				after, loadErr := LoadAppConfig(path)
				return tuiConfigDoneMsg{changed: !reflect.DeepEqual(before, after), err: errors.Join(err, loadErr)}
			})
		},
		login: func() tea.Cmd {
			return tea.ExecProcess(tuiLoginProcess(ctx, executable, opts.repoDir), func(err error) tea.Msg {
				return tuiLoginResult(err)
			})
		},
		workers: workers,
	}
	profile := tuiColorProfile(colorMode, colorprofile.Detect(os.Stdout, os.Environ()))
	m := newReviewTUI(ctx, opts, services, profile >= colorprofile.ANSI)
	prefs, prefsErr := loadTUIPreferences()
	if prefsErr != nil {
		m.notice, m.isError = prefsErr.Error(), true
	} else {
		m.preferences = prefs
	}
	m.refresh(false)
	_, err = tea.NewProgram(m, tea.WithContext(ctx), tea.WithColorProfile(profile)).Run()
	return err
}

func tuiLoginProcess(ctx context.Context, executable, repoDir string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, "auth", "login", "--provider", "chatgpt")
	cmd.Dir = repoDir
	return cmd
}

func tuiLoginResult(err error) tuiLoginDoneMsg {
	if err != nil {
		return tuiLoginDoneMsg{err: err}
	}
	store, err := chatgptauth.DefaultStore()
	if err != nil {
		return tuiLoginDoneMsg{err: err}
	}
	auth, err := store.Selected("")
	if err != nil {
		return tuiLoginDoneMsg{err: err}
	}
	return tuiLoginDoneMsg{planEnabled: auth.PlanEnabled()}
}

// Choose the ASCII wordmark before rendering when the terminal disables color;
// stripping color from background-painted spaces would make it invisible.
func tuiColorProfile(mode string, detected colorprofile.Profile) colorprofile.Profile {
	switch mode {
	case colorModeAlways:
		return colorprofile.TrueColor
	case colorModeNever:
		return colorprofile.ASCII
	default:
		return detected
	}
}
