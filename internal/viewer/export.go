// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"path/filepath"
)

// ExportSession renders one persisted session as a single self-contained HTML
// document and writes it to out. It is the offline counterpart of the viewer's
// GET /r/{repo}/{sessionID} handler: the same template and the same data, with
// both /static/ assets inlined so the result opens over file:// with no network
// access at all.
//
// The two assets are handed to the template as template.CSS and template.JS
// rather than as plain strings. html/template refuses to interpolate an untyped
// string into a <style> or <script> context: the stylesheet would render as the
// literal ZgotmplZ and the script as a JSON string literal, both silently. The
// embedded assets are trusted by construction — they ship in the binary — so
// the typed conversion is the correct escape hatch here, matching the way
// cmd/cover inlines its own assets.
//
// The page is rendered fully in memory before a single byte reaches out.
// Callers pass a writer that creates its target lazily on the first write, so
// streaming a half-rendered page into it would leave a truncated file behind
// when Execute fails partway through.
func ExportSession(out io.Writer, root, encodedRepo, sessionID string) error {
	vs, err := LoadSession(root, encodedRepo, sessionID)
	if err != nil {
		return fmt.Errorf("load session %q: %w", sessionID, err)
	}

	css, err := assets.ReadFile("static/style.css")
	if err != nil {
		return fmt.Errorf("read embedded stylesheet: %w", err)
	}
	js, err := assets.ReadFile("static/session.js")
	if err != nil {
		return fmt.Errorf("read embedded script: %w", err)
	}

	tmpl, err := parseTemplate("session.html")
	if err != nil {
		return fmt.Errorf("parse session template: %w", err)
	}

	// Same display name the live handler derives, so the exported page and the
	// served one carry an identical breadcrumb.
	name := filepath.Base(vs.Summary.CWD)
	if name == "." || name == "" {
		name = encodedRepo
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, sessionPageData{
		EncodedRepo: encodedRepo,
		RepoName:    name,
		Session:     vs,
		Static:      true,
		InlineCSS:   template.CSS(css),
		InlineJS:    template.JS(js),
	}); err != nil {
		return fmt.Errorf("render session %q: %w", sessionID, err)
	}

	_, err = out.Write(buf.Bytes())
	return err
}
