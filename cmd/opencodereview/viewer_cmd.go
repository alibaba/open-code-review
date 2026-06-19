package main

import (
	"fmt"
	"os"

	"github.com/open-code-review/open-code-review/internal/viewer"
)

type viewerOptions struct {
	addr       string
	reviewsDir string
	showHelp   bool
}

func parseViewerFlags(args []string) (viewerOptions, error) {
	a := newOcrFlagSet("ocr viewer")

	opts := viewerOptions{}
	a.StringVar(&opts.addr, "addr", "localhost:5483", "listen address")
	a.StringVar(&opts.reviewsDir, "reviews-dir", "", "review result storage root (default: ~/.opencodereview/reviews)")

	if err := a.Parse(args); err != nil {
		return opts, fmt.Errorf("parse flags: %w", err)
	}

	opts.showHelp = a.showHelp
	if opts.reviewsDir == "" {
		opts.reviewsDir = os.Getenv("OCR_REVIEWS_DIR")
	}
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

	fmt.Printf("Open Code Review Viewer starting on http://%s\n", opts.addr)
	return viewer.StartServerWithOptions(viewer.ServerOptions{
		Addr:       opts.addr,
		ReviewsDir: opts.reviewsDir,
	})
}

func printViewerUsage() {
	fmt.Println(`Session history WebUI viewer.

Usage:
  ocr viewer [flags]
  ocr v [flags]              (alias)

Flags:
  --addr <address>           listen address (default: localhost:5483)
  --reviews-dir <path>       review result storage root (default: ~/.opencodereview/reviews)

Examples:
  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
  ocr viewer --reviews-dir /ocr-data/reviews`)
}
