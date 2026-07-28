package main

import (
	"fmt"

	"github.com/alibaba/open-code-review/internal/viewer"
)

type viewerOptions struct {
	addr     string
	showHelp bool
}

func parseViewerFlags(args []string) (viewerOptions, error) {
	a := newOcrFlagSet("ocr viewer")

	opts := viewerOptions{}
	a.StringVar(&opts.addr, "addr", "localhost:5483", "listen address")

	if err := a.Parse(args); err != nil {
		return opts, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	return opts, nil
}

func runViewer(args []string) error {
	opts, err := parseViewerFlags(args)
	if err != nil {
		return err
	}
	if opts.showHelp {
		printViewerUsage()
		return nil
	}

	fmt.Printf("Open Code Review Viewer starting on http://%s\n", viewer.DisplayAddr(opts.addr))
	return viewer.StartServer(opts.addr)
}

func printViewerUsage() {
	fmt.Println(`Session history WebUI viewer.

Usage:
  ocr viewer [flags]
  ocr v [flags]              (alias)

Flags:
  --addr <address>           listen address (default: localhost:5483)

Examples:
  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000`)
}
