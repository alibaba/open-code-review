// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish|powershell]",
	Short:     "Generate shell completion scripts",
	Long:      completionLongHelp,
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", args[0])
		}
	},
}

const completionLongHelp = `Generate shell completion scripts for OCR.

To load completions:

Bash:
  $ source <(ocr completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ ocr completion bash > /etc/bash_completion.d/ocr
  # macOS:
  $ ocr completion bash > $(brew --prefix)/etc/bash_completion.d/ocr

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ ocr completion zsh > "${fpath[1]}/_ocr"
  # You will need to start a new shell for this setup to take effect.

Fish:
  $ ocr completion fish | source
  # To load completions for each session, execute once:
  $ ocr completion fish > ~/.config/fish/completions/ocr.fish

PowerShell:
  PS> ocr completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> ocr completion powershell > ocr.ps1
  # and source this file from your PowerShell profile.`
