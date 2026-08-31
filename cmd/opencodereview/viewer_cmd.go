// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"fmt"
	"net"
	"strconv"

	"github.com/alibaba/open-code-review/internal/viewer"
	"github.com/spf13/cobra"
)

type viewerOptions struct {
	bind string
	port int
}

var viewerOpts viewerOptions

var viewerCmd = &cobra.Command{
	Use:     "viewer [flags]",
	Aliases: []string{"v"},
	Short:   "Start the WebUI session viewer",
	Long:    "Session history WebUI viewer.",
	Args:    cobra.NoArgs,
	Example: `  ocr viewer                      # start on localhost:5483
  ocr viewer --bind 0.0.0.0 -p 8080  # bind to all interfaces on port 8080`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := viewerListenAddr(viewerOpts.bind, viewerOpts.port)
		fmt.Printf("Open Code Review Viewer starting on http://%s\n", viewer.DisplayAddr(addr))
		return viewer.StartServer(addr)
	},
}

func init() {
	viewerCmd.Flags().StringVar(&viewerOpts.bind, "bind", "localhost", "interface to bind to")
	viewerCmd.Flags().IntVarP(&viewerOpts.port, "port", "p", 5483, "port to listen on")
}

func viewerListenAddr(bind string, port int) string {
	return net.JoinHostPort(bind, strconv.Itoa(port))
}
