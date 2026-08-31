// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"github.com/alibaba/open-code-review/internal/viewer"
	"github.com/spf13/cobra"
)

type viewerOptions struct {
	addr string
}

var viewerOpts viewerOptions

var viewerCmd = &cobra.Command{
	Use:     "viewer [flags]",
	Aliases: []string{"v"},
	Short:   "Start the WebUI session viewer",
	Long:    "Session history WebUI viewer.",
	Args:    cobra.NoArgs,
	Example: `  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return viewer.StartServer(viewerOpts.addr)
	},
}

func init() {
	viewerCmd.Flags().StringVar(&viewerOpts.addr, "addr", "localhost:5483", "listen address")
}
