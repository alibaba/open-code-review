package main

import "github.com/spf13/cobra"

// parseReviewFlags provides test compatibility: parses args through a fresh
// cobra command instance and returns the resulting reviewOptions.
func parseReviewFlags(args []string) (reviewOptions, error) {
	var opts reviewOptions
	cmd := &cobra.Command{
		Use:           "review",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateReviewOptions(&opts)
		},
	}
	registerReviewFlags(cmd, &opts)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return opts, err
}

// parseConfigArgs provides test compatibility for config argument parsing.
type configAction struct {
	subCmd string
	key    string
	value  string
}

func parseConfigArgs(args []string) (configAction, error) {
	if len(args) == 0 {
		return configAction{}, &configParseError{"usage: ocr config set <key> <value>\ne.g., ocr config set llm.model claude-opus-4-6"}
	}

	subCmd := args[0]
	switch subCmd {
	case "set":
		if len(args) < 3 {
			return configAction{}, &configParseError{"usage: ocr config set <key> <value>\ne.g., ocr config set llm.model claude-opus-4-6"}
		}
		return configAction{subCmd: "set", key: args[1], value: args[2]}, nil
	case "unset":
		if len(args) < 2 {
			return configAction{}, &configParseError{"usage: ocr config unset <provider|custom_providers.<name>|mcp_servers.<name>>\nexamples:\n  ocr config unset provider\n  ocr config unset custom_providers.my-provider\n  ocr config unset mcp_servers.github"}
		}
		return configAction{subCmd: "unset", key: args[1]}, nil
	default:
		return configAction{}, &configParseError{"unknown config sub-command: " + subCmd + "\nAvailable: set, unset, provider, model"}
	}
}

// parseScanFlags provides test compatibility: parses args through a fresh
// cobra command instance and returns the resulting scanOptions.
func parseScanFlags(args []string) (scanOptions, error) {
	var opts scanOptions
	cmd := &cobra.Command{
		Use:           "scan",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateScanOptions(&opts)
		},
	}
	registerScanFlags(cmd, &opts)
	cmd.SetArgs(args)
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {})
	err := cmd.Execute()
	return opts, err
}

type configParseError struct{ msg string }

func (e *configParseError) Error() string { return e.msg }

// runSessionListCompat provides test compatibility for old-style runSessionList([]string{...}) calls.
func runSessionListCompat(args []string) error {
	cmd := &cobra.Command{
		Use:           "list",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, a []string) error {
			return runSessionList()
		},
	}
	cmd.Flags().StringVar(&sessionListRepoDir, "repo", "", "")
	cmd.Flags().BoolVar(&sessionListJSON, "json", false, "")
	cmd.Flags().IntVar(&sessionListLimit, "limit", 20, "")
	cmd.SetArgs(args)
	return cmd.Execute()
}

// runSessionShowCompat provides test compatibility for old-style runSessionShow([]string{...}) calls.
func runSessionShowCompat(args []string) error {
	cmd := &cobra.Command{
		Use:           "show",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			return runSessionShow(a[0])
		},
	}
	cmd.Flags().StringVar(&sessionShowRepoDir, "repo", "", "")
	cmd.Flags().BoolVar(&sessionShowJSON, "json", false, "")
	cmd.SetArgs(args)
	return cmd.Execute()
}

// runSessionCommentsCompat provides test compatibility for old-style runSessionComments([]string{...}) calls.
func runSessionCommentsCompat(args []string) error {
	cmd := &cobra.Command{
		Use:           "comments",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			return runSessionComments(a[0])
		},
	}
	cmd.Flags().StringVar(&sessionCommentsRepoDir, "repo", "", "")
	cmd.Flags().BoolVar(&sessionCommentsJSON, "json", false, "")
	cmd.Flags().StringVar(&sessionCommentsSeverity, "severity", "", "")
	cmd.Flags().StringVar(&sessionCommentsCategory, "category", "", "")
	cmd.SetArgs(args)
	return cmd.Execute()
}

// runSession provides test compatibility for the old dispatch function.
func runSession(args []string) error {
	cmd := &cobra.Command{Use: "session", SilenceUsage: true, SilenceErrors: true}
	listCmd := &cobra.Command{
		Use: "list", Aliases: []string{"ls"},
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, a []string) error { return runSessionList() },
	}
	listCmd.Flags().StringVar(&sessionListRepoDir, "repo", "", "")
	listCmd.Flags().BoolVar(&sessionListJSON, "json", false, "")
	listCmd.Flags().IntVar(&sessionListLimit, "limit", 20, "")
	showCmd := &cobra.Command{
		Use: "show", Args: cobra.ExactArgs(1),
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, a []string) error { return runSessionShow(a[0]) },
	}
	showCmd.Flags().StringVar(&sessionShowRepoDir, "repo", "", "")
	showCmd.Flags().BoolVar(&sessionShowJSON, "json", false, "")
	cmd.AddCommand(listCmd, showCmd)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// Ensure configParseError implements the error interface.
var _ error = (*configParseError)(nil)

// runConfig provides test compatibility: dispatches config subcommands.
func runConfig(args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "provider":
		if len(args) != 1 {
			return &configParseError{"config provider does not accept arguments; use 'ocr config set provider <name>' for non-interactive setup"}
		}
		return runConfigProvider()
	case "model":
		if len(args) != 1 {
			return &configParseError{"config model does not accept arguments; use 'ocr config set model <name>' for non-interactive setup"}
		}
		return runConfigModel()
	}

	action, err := parseConfigArgs(args)
	if err != nil {
		return err
	}

	switch action.subCmd {
	case "set":
		return runConfigSet(action.key, action.value)
	case "unset":
		return runConfigUnset(action.key)
	default:
		return &configParseError{"unknown config sub-command: " + action.subCmd}
	}
}
