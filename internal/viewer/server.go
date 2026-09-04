// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/*.html static/style.css static/session.js static/repos.js
var assets embed.FS

// StartServer binds addr and serves until the listener fails. openMode is one
// of OpenAuto, OpenAlways or OpenNever; callers should have run
// ValidateOpenMode first, and anything unrecognized behaves as OpenAuto.
func StartServer(addr, openMode string) error {
	root, err := SessionsRoot()
	if err != nil {
		return fmt.Errorf("resolve sessions root: %w", err)
	}

	mux := newMux(root)

	// Wrap the mux with a Host-header allowlist. Without this, any web page
	// the user visits can DNS-rebind its origin to 127.0.0.1 and read the
	// session JSONL exposed by this viewer (which contains LLM request bodies
	// = source code being reviewed and the LLM's analysis of it).
	allowed := resolveAllowedHostsFromEnv(addr)
	guarded := hostGuard(allowed, mux)

	// Outermost layer: set defense-in-depth security headers on every response.
	handler := securityHeaders(guarded)

	srv := &http.Server{
		Handler: handler,
	}

	// Bind before printing or opening anything: once Listen returns, early
	// connections queue in the accept backlog instead of being refused, so the
	// browser cannot outrun the server.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	// srv.Serve takes ownership and closes ln itself; this covers the
	// early-return path below and any future one. Close is idempotent enough
	// here — the second call just reports ErrClosed, which nothing reads.
	defer ln.Close()

	url, err := displayURL(addr, ln.Addr().String())
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	autoOpen, suppressed := shouldAutoOpen(openMode)
	if suppressed != "" {
		fmt.Printf("Viewer ready: %s (browser not opened: %s)\n", url, suppressed)
	} else {
		fmt.Printf("Viewer ready: %s\n", url)
	}
	if autoOpen {
		go func() {
			if err := openBrowser(url); err != nil {
				browserWarnf("could not open browser: %v", err)
			}
		}()
	}

	return <-serveErr
}

// newMux builds the viewer's routing table against a sessions root. The
// viewer is read-only: the document routes are registered with GET-only
// patterns (which also serve HEAD), so the ServeMux itself answers any other
// method with 405 + Allow before a handler runs. The root pattern matches
// exactly "/" via {$}; every other unmatched path gets the ServeMux's 404.
// New routes must register method-qualified patterns to keep this contract
// testable (TestMux_HasNoWriteRoutes).
func newMux(root string) *http.ServeMux {
	mux := http.NewServeMux()

	// Static assets.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS()))))

	// Routes
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		handleRepos(w, r, root)
	})
	mux.HandleFunc("GET /r/{repo}", func(w http.ResponseWriter, r *http.Request) {
		repo := r.PathValue("repo")
		if strings.Contains(repo, "..") || strings.Contains(repo, "/") {
			http.Error(w, "invalid repo path", http.StatusBadRequest)
			return
		}
		handleSessions(w, r, root, repo)
	})
	mux.HandleFunc("GET /r/{repo}/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		repo := r.PathValue("repo")
		sid := r.PathValue("sessionID")
		if strings.Contains(repo, "..") || strings.Contains(sid, "..") {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		handleSession(w, r, root, repo, sid)
	})

	return mux
}

// displayURL builds the URL to print and hand to the browser.
//
// The host comes from the requested address, not from the listener: net.Listen
// resolves a hostname to an IP literal, while the Host allowlist in hostGuard is
// built from the requested address (resolveAllowedHostsFromEnv). Using the
// resolved form makes the two disagree, so `ocr viewer --addr box.local:5483`
// would auto-open http://192.168.1.10:5483 and land on "403 forbidden host".
//
// The port comes from the listener so `--addr :0` reports the port the kernel
// actually assigned rather than the literal 0.
func displayURL(requestedAddr, listenerAddr string) (string, error) {
	_, port, err := net.SplitHostPort(listenerAddr)
	if err != nil {
		return "", fmt.Errorf("parse listener addr %q: %w", listenerAddr, err)
	}
	return "http://" + DisplayAddr(net.JoinHostPort(splitBindHost(requestedAddr), port)), nil
}

var cstZone = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

func formatTime(t time.Time) string {
	return t.In(cstZone).Format("2006-01-02 15:04")
}

// CommentFileGroup groups review comments by file path for template rendering.
type CommentFileGroup struct {
	FilePath string
	Comments []*ReviewComment
}

// SeverityCount holds counts for each severity level.
type SeverityCount struct {
	Critical int
	High     int
	Medium   int
	Low      int
}

// CategoryCount holds counts for each review comment category.
type CategoryCount struct {
	Bug             int
	Security        int
	Performance     int
	Maintainability int
	Test            int
	Style           int
	Documentation   int
	Other           int
}

var knownCommentCategories = map[string]struct{}{
	"bug":             {},
	"security":        {},
	"performance":     {},
	"maintainability": {},
	"test":            {},
	"style":           {},
	"documentation":   {},
	"other":           {},
}

func normalizedCommentCategory(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	if _, ok := knownCommentCategories[category]; ok {
		return category
	}
	return "other"
}

func normalizedCommentSeverity(severity string) string {
	return strings.ToLower(strings.TrimSpace(severity))
}

func categoryCounts(comments []*ReviewComment) CategoryCount {
	var counts CategoryCount
	for _, comment := range comments {
		switch normalizedCommentCategory(comment.Category) {
		case "bug":
			counts.Bug++
		case "security":
			counts.Security++
		case "performance":
			counts.Performance++
		case "maintainability":
			counts.Maintainability++
		case "test":
			counts.Test++
		case "style":
			counts.Style++
		case "documentation":
			counts.Documentation++
		default:
			counts.Other++
		}
	}
	return counts
}

func severityCounts(comments []*ReviewComment) SeverityCount {
	var counts SeverityCount
	for _, comment := range comments {
		switch strings.ToLower(strings.TrimSpace(comment.Severity)) {
		case "critical":
			counts.Critical++
		case "high":
			counts.High++
		case "medium":
			counts.Medium++
		case "low":
			counts.Low++
		}
	}
	return counts
}

func parseTemplate(name string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"formatDuration": formatDuration,
		"formatTime":     formatTime,
		"truncate":       truncateText,
		"formatNumber":   formatNumber,
		"add":            func(a, b int) int { return a + b },
		"cardCount": func(tasks map[TaskType][]*TaskCard) int {
			n := 0
			for _, cards := range tasks {
				n += len(cards)
			}
			return n
		},
		"taskTypeClass": func(tt TaskType) string {
			switch tt {
			case PlanTask:
				return "task-plan"
			case MainTask:
				return "task-main"
			case MemoryCompressionTask:
				return "task-memory"
			case ReLocationTask:
				return "task-relocation"
			case GroupingTask:
				return "task-grouping"
			default:
				return "task-default"
			}
		},
		"sessionTaskLabel": func(fp string) string {
			switch fp {
			case "__grouping__":
				return "File Grouping"
			default:
				return fp
			}
		},
		"orderedTasks": func(tasks map[TaskType][]*TaskCard) []struct {
			Type  TaskType
			Cards []*TaskCard
		} {
			order := []TaskType{PlanTask, MainTask, ReLocationTask, MemoryCompressionTask, GroupingTask}
			var result []struct {
				Type  TaskType
				Cards []*TaskCard
			}
			for _, tt := range order {
				if cards, ok := tasks[tt]; ok {
					result = append(result, struct {
						Type  TaskType
						Cards []*TaskCard
					}{tt, cards})
				}
			}
			for tt, cards := range tasks {
				if tt != PlanTask && tt != MainTask && tt != ReLocationTask && tt != MemoryCompressionTask && tt != GroupingTask {
					result = append(result, struct {
						Type  TaskType
						Cards []*TaskCard
					}{tt, cards})
				}
			}
			return result
		},
		"groupCommentsByFile": func(comments []*ReviewComment) []CommentFileGroup {
			index := make(map[string]int)
			var groups []CommentFileGroup
			for _, c := range comments {
				idx, ok := index[c.FilePath]
				if !ok {
					idx = len(groups)
					index[c.FilePath] = idx
					groups = append(groups, CommentFileGroup{FilePath: c.FilePath})
				}
				groups[idx].Comments = append(groups[idx].Comments, c)
			}
			return groups
		},
		"severityCounts":  severityCounts,
		"categoryCounts":  categoryCounts,
		"commentCategory": normalizedCommentCategory,
		"commentSeverity": normalizedCommentSeverity,
		"severityClass": func(s string) string {
			switch normalizedCommentSeverity(s) {
			case "critical":
				return "severity-critical"
			case "high":
				return "severity-high"
			case "medium":
				return "severity-medium"
			case "low":
				return "severity-low"
			default:
				return "severity-default"
			}
		},
		"categoryClass": func(s string) string {
			switch normalizedCommentCategory(s) {
			case "bug":
				return "cat-bug"
			case "security":
				return "cat-security"
			case "performance":
				return "cat-performance"
			case "maintainability":
				return "cat-maintainability"
			case "test":
				return "cat-test"
			case "style":
				return "cat-style"
			case "documentation":
				return "cat-documentation"
			case "other":
				return "cat-other"
			default:
				return "cat-default"
			}
		},
	}
	content, err := assets.ReadFile("templates/" + name)
	if err != nil {
		return nil, err
	}
	return template.New(name).Funcs(funcMap).Parse(string(content))
}

func truncateText(n int, s string) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	tmpl, err := parseTemplate(name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		// Partially written — just log
		fmt.Printf("[ocr] template execution error: %v\n", err)
	}
}

func staticFS() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

func formatNumber(n int) string {
	var display string
	switch {
	case n >= 1_000_000:
		if n%1_000_000 == 0 {
			display = fmt.Sprintf("%dM", n/1_000_000)
		} else {
			display = trimFloatSuffix(fmt.Sprintf("%.2fM", float64(n)/1_000_000))
		}
	case n >= 1_000:
		if n%1_000 == 0 {
			display = fmt.Sprintf("%dK", n/1_000)
		} else {
			display = trimFloatSuffix(fmt.Sprintf("%.2fK", float64(n)/1_000))
		}
	default:
		display = strconv.Itoa(n)
	}
	return display
}

// trimFloatSuffix removes trailing zeros and the trailing dot from a
// floating-point string like "1.10K" → "1.1K", "1.00K" → "1K".
func trimFloatSuffix(s string) string {
	// Find the dot position before the suffix (K/M).
	// Input is always "%d.%dX" or "%dX".
	dot := strings.LastIndexByte(s, '.')
	if dot < 0 {
		return s
	}
	// Find the suffix letter (K or M) — it's always the last character.
	suffix := s[len(s)-1]
	mantissa := s[:len(s)-1] // strip suffix

	// Trim trailing zeros from the fractional part.
	i := len(mantissa) - 1
	for i >= 0 && mantissa[i] == '0' {
		i--
	}
	if i >= 0 && mantissa[i] == '.' {
		i-- // also trim the dot if whole fractional part was zeros
	}
	return mantissa[:i+1] + string(suffix)
}

func formatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", seconds)
	}
	minutes := int(d.Minutes())
	sec := int(d.Seconds()) - minutes*60
	return fmt.Sprintf("%dm%ds", minutes, sec)
}
