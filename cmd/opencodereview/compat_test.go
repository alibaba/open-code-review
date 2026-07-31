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

// runConfig provides test compatibility: dispatches config subcommands through a
// fresh cobra command tree that mirrors the production config command, so the
// test compat layer cannot drift from production routing.
func runConfig(args []string) error {
	cmd := &cobra.Command{Use: "config", SilenceUsage: true, SilenceErrors: true}
	setCmd := &cobra.Command{
		Use: "set", Args: cobra.ExactArgs(2),
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, a []string) error { return runConfigSet(a[0], a[1]) },
	}
	unsetCmd := &cobra.Command{
		Use: "unset", Args: cobra.ExactArgs(1),
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, a []string) error { return runConfigUnset(a[0]) },
	}
	providerCmd := &cobra.Command{
		Use: "provider", Args: cobra.NoArgs,
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, a []string) error { return runConfigProvider() },
	}
	modelCmd := &cobra.Command{
		Use: "model", Args: cobra.NoArgs,
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, a []string) error { return runConfigModel() },
	}
	cmd.AddCommand(setCmd, unsetCmd, providerCmd, modelCmd)
	// A nil args slice makes cobra fall back to os.Args; pass an explicit empty
	// slice so `runConfig(nil)` shows usage instead of reading the test binary's args.
	if args == nil {
		args = []string{}
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}
