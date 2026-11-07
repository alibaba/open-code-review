// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func oauthExecutable() (string, error) {
	name, env := "claude", "OCR_CLAUDE_PATH"
	if path := strings.TrimSpace(os.Getenv(env)); path != "" {
		name = path
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("official %s executable not found; install it or set %s", name, env)
	}
	return filepath.Abs(path)
}

func oauthEnvironment() []string {
	blocked := map[string]bool{
		"ANTHROPIC_API_KEY": true, "ANTHROPIC_AUTH_TOKEN": true,
		"ANTHROPIC_BASE_URL": true, "CLAUDE_CODE_OAUTH_TOKEN": true,
		"ANTHROPIC_CUSTOM_HEADERS": true, "CLAUDE_CODE_EXTRA_BODY": true,
		"CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR": true, "CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR": true,
		"CLAUDE_CODE_USE_BEDROCK": true, "CLAUDE_CODE_USE_VERTEX": true,
		"CLAUDE_CODE_USE_FOUNDRY": true, "CLAUDE_CODE_SIMPLE": true,
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !blocked[strings.ToUpper(key)] {
			env = append(env, entry)
		}
	}
	return env
}

func RunOAuthAuth(ctx context.Context, provider, action string, stdin io.Reader, stdout, stderr io.Writer) error {
	preset, ok := LookupProvider(provider)
	if !ok || !IsOAuthProtocol(preset.Protocol) {
		return fmt.Errorf("OAuth authentication supports anthropic-oauth")
	}
	if action != "login" && action != "status" && action != "logout" {
		return fmt.Errorf("unsupported OAuth action %q", action)
	}
	path, err := oauthExecutable()
	if err != nil {
		return err
	}
	args := []string{"auth", action}
	if action == "login" {
		args = append(args, "--claudeai")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = oauthEnvironment()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", provider, action, err)
	}
	return nil
}
