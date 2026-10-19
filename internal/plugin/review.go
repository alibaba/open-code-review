// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/model"
)

type ReviewRequest struct {
	Version    int            `json:"version"`
	Method     string         `json:"method"`
	Stage      string         `json:"stage"`
	Paths      []string       `json:"paths,omitempty"`
	Severities []string       `json:"severities,omitempty"`
	Categories []string       `json:"categories,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

type ReviewCompletion struct {
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ReviewTask struct {
	Instructions string           `json:"instructions"`
	Input        string           `json:"input"`
	Completion   ReviewCompletion `json:"completion"`
}

type ReviewResponse struct {
	Version  int                `json:"version"`
	Task     *ReviewTask        `json:"task,omitempty"`
	Accepted *bool              `json:"accepted,omitempty"`
	Complete *bool              `json:"complete,omitempty"`
	Feedback string             `json:"feedback,omitempty"`
	Report   json.RawMessage    `json:"report,omitempty"`
	Comments []model.LlmComment `json:"comments,omitempty"`
	Error    string             `json:"error,omitempty"`
}

func ReviewCommand(p Installed) (string, []string, error) {
	if p.Manifest.Review == nil {
		return "", nil, fmt.Errorf("plugin %q has no review contribution", p.Manifest.ID)
	}
	if err := p.Manifest.Validate(); err != nil {
		return "", nil, err
	}
	if p.Manifest.Review.retired() {
		return "", nil, fmt.Errorf("plugin %q uses the retired engine/prompt format; update it with ocr plugin update %s", p.Manifest.ID, p.Manifest.ID)
	}
	args := make([]string, len(p.Manifest.Review.Args))
	for i, arg := range p.Manifest.Review.Args {
		args[i] = ExpandCommandValue(arg, p.Dir)
	}
	return ExpandCommandValue(p.Manifest.Review.Command, p.Dir), args, nil
}

// ExpandCommandValue replaces package/platform placeholders without expanding
// shell syntax, environment variables, or placeholders inside substituted paths.
func ExpandCommandValue(value, directory string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return strings.NewReplacer("${plugin_dir}", directory, "${os}", runtime.GOOS, "${arch}", runtime.GOARCH, "${exe}", ext).Replace(value)
}

type ReviewClient struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output io.ReadCloser
	lines  *bufio.Scanner
	stderr reviewStderr
	cancel context.CancelFunc
}

// StartReview launches a separate workflow process for each file group. Requests
// carry task metadata and results; OCR does not send its configured credentials.
func StartReview(ctx context.Context, p Installed, repoDir string) (*ReviewClient, error) {
	if err := p.Manifest.CheckCompatibility(HostVersion); err != nil {
		return nil, err
	}
	command, args, err := ReviewCommand(p)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &ReviewClient{cmd: exec.CommandContext(ctx, command, args...), cancel: cancel}
	c.cmd.Dir = repoDir
	c.cmd.Stderr = &c.stderr
	c.cmd.WaitDelay = time.Second
	c.input, err = c.cmd.StdinPipe()
	if err == nil {
		c.output, err = c.cmd.StdoutPipe()
	}
	if err == nil {
		err = c.cmd.Start()
	}
	if err != nil {
		cancel()
		if c.input != nil {
			_ = c.input.Close()
		}
		if c.output != nil {
			_ = c.output.Close()
		}
		return nil, fmt.Errorf("start review plugin %q: %w", p.Manifest.ID, err)
	}
	c.lines = bufio.NewScanner(c.output)
	c.lines.Buffer(make([]byte, 4096), 4<<20)
	// Closing the pipes also releases a blocked call if a child of the plugin
	// inherited stdout and survives cancellation of the executable itself.
	go func() {
		<-ctx.Done()
		_ = c.input.Close()
		_ = c.output.Close()
	}()
	return c, nil
}

// Call is sequential: a plugin must emit one JSON line for each request.
func (c *ReviewClient) Call(req ReviewRequest) (ReviewResponse, error) {
	var res ReviewResponse
	req.Version = 1
	data, err := json.Marshal(req)
	if err == nil && len(data) > 4<<20 {
		err = fmt.Errorf("review request exceeds 4 MiB")
	}
	if err == nil {
		_, err = c.input.Write(append(data, '\n'))
	}
	if err == nil && !c.lines.Scan() {
		err = c.lines.Err()
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
	}
	if err == nil {
		err = decodeStrict(c.lines.Bytes(), &res)
	}
	if err != nil {
		return res, fmt.Errorf("review plugin %s: %w; stderr: %s", req.Method, err, c.stderr.String())
	}
	if res.Version != 1 || res.Error != "" {
		return res, fmt.Errorf("review plugin protocol error (version %d): %s", res.Version, res.Error)
	}
	if req.Method == "begin" {
		if res.Task == nil || strings.TrimSpace(res.Task.Instructions) == "" || strings.TrimSpace(res.Task.Input) == "" || res.Task.Completion.Parameters["type"] != "object" || len(res.Comments) != 0 {
			return res, fmt.Errorf("review begin requires nonempty task instructions and input, and an object completion schema, without comments")
		}
	} else if req.Method == "submit" {
		if res.Accepted == nil || (*res.Accepted && res.Complete == nil) || (!*res.Accepted && (strings.TrimSpace(res.Feedback) == "" || len(res.Comments) != 0)) {
			return res, fmt.Errorf("review submit requires accepted, complete on acceptance, or feedback without comments on rejection")
		}
	}
	return res, nil
}

func (c *ReviewClient) Close() {
	_ = c.input.Close()
	c.cancel()
	_ = c.cmd.Wait()
	_ = c.output.Close()
}

type reviewStderr struct {
	mu   sync.Mutex
	text []byte
}

func (b *reviewStderr) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := min(len(p), 8192-len(b.text))
	b.text = append(b.text, p[:n]...)
	return len(p), nil
}

func (b *reviewStderr) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.text)
}
