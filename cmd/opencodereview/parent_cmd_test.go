package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestParentCommands_UnknownSubcommand verifies that parent commands which
// previously silently printed help and exited 0 now return a non-zero exit
// code with a clear "unknown command" error when given an unrecognized
// subcommand. This is consistent with the root command and leaf commands.
//
// See: https://github.com/alibaba/open-code-review/issues/641
func TestParentCommands_UnknownSubcommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string // substring expected in error message
	}{
		{"session", []string{"session", "bogus"}, "unknown command"},
		{"sessions alias", []string{"sessions", "bogus"}, "unknown command"},
		{"config", []string{"config", "bogus"}, "unknown command"},
		{"delegate", []string{"delegate", "bogus"}, "unknown command"},
		{"delegate alias", []string{"d", "bogus"}, "unknown command"},
		{"llm", []string{"llm", "bogus"}, "unknown command"},
		{"rules", []string{"rules", "bogus"}, "unknown command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clone the global rootCmd tree so we can mutate it in isolation.
			root := cloneRootCmd(t)
			root.SetArgs(tt.args)

			err := root.Execute()
			if err == nil {
				t.Fatalf("expected error for args %v, got nil", tt.args)
			}
			got := err.Error()
			if !strings.Contains(got, tt.want) {
				t.Errorf("expected error containing %q, got %q", tt.want, got)
			}
		})
	}
}

// TestParentCommands_KnownSubcommandStillWorks ensures that adding RunE to
// parent commands does not break legitimate subcommand routing.
func TestParentCommands_KnownSubcommandStillWorks(t *testing.T) {
	// We cannot actually invoke llm test or config provider without a real
	// config, but we can verify that the subcommand tree resolves correctly by
	// using the help flag, which is handled by Cobra before RunE.
	tests := []struct {
		name string
		args []string
	}{
		{"session list help", []string{"session", "list", "--help"}},
		{"session show help", []string{"session", "show", "--help"}},
		{"config set help", []string{"config", "set", "--help"}},
		{"config provider help", []string{"config", "provider", "--help"}},
		{"delegate preview help", []string{"delegate", "preview", "--help"}},
		{"delegate rule help", []string{"delegate", "rule", "--help"}},
		{"llm test help", []string{"llm", "test", "--help"}},
		{"llm providers help", []string{"llm", "providers", "--help"}},
		{"rules check help", []string{"rules", "check", "--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := cloneRootCmd(t)
			root.SetArgs(tt.args)
			// Help output is not treated as an error by Cobra when invoked via --help.
			err := root.Execute()
			if err != nil {
				t.Fatalf("unexpected error for help on known subcommand: %v", err)
			}
		})
	}
}

// TestParentCommands_NoArgsPrintsHelp verifies that invoking a parent
// command with no arguments still prints help and exits 0 (RunE returns
// cmd.Help() which is nil).
func TestParentCommands_NoArgsPrintsHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"session no args", []string{"session"}},
		{"config no args", []string{"config"}},
		{"delegate no args", []string{"delegate"}},
		{"llm no args", []string{"llm"}},
		{"rules no args", []string{"rules"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := cloneRootCmd(t)
			root.SetArgs(tt.args)
			err := root.Execute()
			if err != nil {
				t.Fatalf("expected nil error when parent command has no args (help), got %v", err)
			}
		})
	}
}

// cloneRootCmd creates a deep clone of the global rootCmd tree for isolated
// testing. It preserves SilenceUsage / SilenceErrors so tests do not pollute
// stdout/stderr.
func cloneRootCmd(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{
		Use:           rootCmd.Use,
		Short:         rootCmd.Short,
		Long:          rootCmd.Long,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          rootCmd.RunE,
	}
	root.SetFlagErrorFunc(flagErrorWithSuggestion)
	root.Flags().BoolP("version", "V", false, "version for ocr")

	// Re-register all commands with their current definitions (including our
	// new RunE fields) so the clone is an accurate snapshot.
	root.AddCommand(versionCmd)
	root.AddCommand(reviewCmd)
	root.AddCommand(scanCmd)
	root.AddCommand(delegateCmd)
	root.AddCommand(sessionCmd)
	root.AddCommand(configCmd)
	root.AddCommand(llmCmd)
	root.AddCommand(rulesCmd)
	root.AddCommand(viewerCmd)
	root.AddCommand(completionCmd)
	return root
}
