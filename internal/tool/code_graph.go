package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	codeGraphTimeout       = 15 * time.Second
	codeGraphDetectTimeout = 3 * time.Second
	codeGraphMaxOutput     = 12000
	codeGraphMaxFiles      = 8
	codeGraphDefaultFiles  = 4
	codeGraphMaxLimit      = 30
	codeGraphDefaultLimit  = 12
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// CodeGraphProvider retrieves structural code context from an optional external
// CodeGraph installation. It is intentionally CLI-backed so OCR does not take a
// hard dependency on CodeGraph's database schema or Go libraries.
type CodeGraphProvider struct {
	RepoDir string
	BinPath string
}

// CodeGraphAvailability describes whether the optional CodeGraph integration can
// be exposed to the model for this review run.
type CodeGraphAvailability struct {
	Available bool
	BinPath   string
	Version   string
	Reason    string
}

func NewCodeGraph(repoDir, binPath string) *CodeGraphProvider {
	return &CodeGraphProvider{RepoDir: repoDir, BinPath: binPath}
}

func (p *CodeGraphProvider) Tool() Tool { return CodeGraph }

func (p *CodeGraphProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	mode := stringArg(args, "mode")
	if mode == "" {
		mode = "explore"
	}
	query := strings.TrimSpace(stringArg(args, "query"))
	if query == "" {
		return "Error: query is required", nil
	}

	limit := intArg(args, "limit", codeGraphDefaultLimit)
	if limit <= 0 || limit > codeGraphMaxLimit {
		limit = codeGraphDefaultLimit
	}
	maxFiles := intArg(args, "max_files", codeGraphDefaultFiles)
	if maxFiles <= 0 {
		maxFiles = codeGraphDefaultFiles
	}
	if maxFiles > codeGraphMaxFiles {
		maxFiles = codeGraphMaxFiles
	}

	cmdArgs := []string{}
	switch mode {
	case "search":
		cmdArgs = []string{"query", "-p", p.RepoDir, "-l", strconv.Itoa(limit)}
		if kind := strings.TrimSpace(stringArg(args, "kind")); kind != "" {
			cmdArgs = append(cmdArgs, "-k", kind)
		}
		cmdArgs = append(cmdArgs, query)
	case "explore":
		cmdArgs = []string{"explore", "-p", p.RepoDir, "--max-files", strconv.Itoa(maxFiles), query}
	case "callers":
		cmdArgs = []string{"callers", "-p", p.RepoDir, "-l", strconv.Itoa(limit), query}
	case "callees":
		cmdArgs = []string{"callees", "-p", p.RepoDir, "-l", strconv.Itoa(limit), query}
	case "impact":
		cmdArgs = []string{"impact", "-p", p.RepoDir, query}
	default:
		return fmt.Sprintf("Error: unsupported mode %q. Supported modes: search, explore, callers, callees, impact", mode), nil
	}

	out, err := p.run(ctx, cmdArgs...)
	if err != nil {
		return "", fmt.Errorf("code_graph_context failed: %w", err)
	}
	return out, nil
}

func (p *CodeGraphProvider) run(parentCtx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parentCtx, codeGraphTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.BinPath, args...)
	cmd.Dir = p.RepoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		return "code_graph_context timed out. Try using mode=search with a specific symbol, or reduce max_files/limit.", nil
	}

	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())
	if err != nil {
		if out == "" && errOut == "" {
			return "Error: codegraph command failed", nil
		}
		if out == "" {
			return "Error: " + stripANSI(errOut), nil
		}
	}
	if errOut != "" {
		out += "\nWarning: " + stripANSI(errOut)
	}
	out = stripANSI(out)
	if len(out) > codeGraphMaxOutput {
		out = out[:codeGraphMaxOutput] + "\n\n[truncated: CodeGraph output exceeded tool limit]"
	}
	if out == "" {
		return "No structural context found", nil
	}
	return out, nil
}

// DetectCodeGraph checks whether CodeGraph can be used for repoDir. A negative
// result means the tool definition should be hidden from the model entirely.
func DetectCodeGraph(repoDir string) CodeGraphAvailability {
	dbPath := filepath.Join(repoDir, ".codegraph", "codegraph.db")
	if _, err := os.Stat(dbPath); err != nil {
		return CodeGraphAvailability{Reason: ".codegraph/codegraph.db not found"}
	}

	binPath, err := exec.LookPath("codegraph")
	if err != nil {
		return CodeGraphAvailability{Reason: "codegraph executable not found in PATH"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), codeGraphDetectTimeout)
	defer cancel()
	versionOut, err := exec.CommandContext(ctx, binPath, "version").Output()
	if ctx.Err() != nil {
		return CodeGraphAvailability{Reason: "codegraph version check timed out"}
	}
	if err != nil {
		return CodeGraphAvailability{Reason: "codegraph version check failed"}
	}
	version := strings.TrimSpace(string(versionOut))
	if !isSupportedCodeGraphVersion(version) {
		return CodeGraphAvailability{Version: version, Reason: "unsupported codegraph version"}
	}

	ctx, cancel = context.WithTimeout(context.Background(), codeGraphDetectTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "status", repoDir)
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return CodeGraphAvailability{BinPath: binPath, Version: version, Reason: "codegraph status timed out"}
		}
		return CodeGraphAvailability{BinPath: binPath, Version: version, Reason: "codegraph status failed"}
	}

	return CodeGraphAvailability{Available: true, BinPath: binPath, Version: version}
}

func isSupportedCodeGraphVersion(version string) bool {
	if version == "" {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) == 0 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	return major == 1
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

func intArg(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return fallback
	}
}

func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}
