package main

import (
	"fmt"

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
	Example: `  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Open Code Review Viewer starting on http://%s\n", viewer.DisplayAddr(viewerOpts.addr))
		return viewer.StartServer(viewerOpts.addr)
	},
}

func init() {
	viewerCmd.Flags().StringVar(&viewerOpts.addr, "addr", "localhost:5483", "listen address")
}
