// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"github.com/alibaba/open-code-review/internal/viewer"
	"github.com/spf13/cobra"
)

type viewerOptions struct {
	addr string
	open string
}

var viewerOpts viewerOptions

var viewerCmd = &cobra.Command{
	Use:     "viewer [flags]",
	Aliases: []string{"v"},
	Short:   "Start the WebUI session viewer",
	Long:    "Session history WebUI viewer.",
	Args:    cobra.NoArgs,
	Example: `  ocr viewer                     # start and open the browser
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
  ocr viewer --open=never        # just print the URL
  ocr viewer --open=always       # force it when auto declines (piped output, WSL)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := viewer.ValidateOpenMode(viewerOpts.open); err != nil {
			return err
		}
		return viewer.StartServer(viewerOpts.addr, viewerOpts.open)
	},
}

func init() {
	viewerCmd.Flags().StringVar(&viewerOpts.addr, "addr", "localhost:5483", "listen address")
	viewerCmd.Flags().StringVar(&viewerOpts.open, "open", viewer.OpenAuto,
		"when to open the browser: auto (only on a local terminal with a display), always, or never")
	viewerCmd.RegisterFlagCompletionFunc("open", completeEnum(viewer.OpenAuto, viewer.OpenAlways, viewer.OpenNever))
}
