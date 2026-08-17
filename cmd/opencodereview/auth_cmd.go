// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/spf13/cobra"
)

var (
	loginProvider  string
	loginNoBrowser bool
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in to an LLM account",
	Long:  "Sign in to an LLM account with its official OAuth flow.",
	Example: `  ocr login --provider openai
  ocr login --provider openai --no-browser`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(loginProvider) != llm.OpenAIAccountProviderName && loginProvider != "openai" {
			return fmt.Errorf("unsupported login provider %q; supported provider: openai", loginProvider)
		}
		return runOpenAILogin()
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginProvider, "provider", "openai", "account provider to sign in")
	loginCmd.Flags().BoolVar(&loginNoBrowser, "no-browser", false, "print the authorization URL without opening a browser")
}

type openAIOAuthCallback struct {
	code  string
	state string
	err   string
}

func runOpenAILogin() error {
	verifier, challenge, state, err := llm.GenerateOpenAIPKCE()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(llm.OpenAIAccountDefaultPort)))
	if err != nil {
		return fmt.Errorf("listen for OpenAI OAuth callback on port %d: %w; close the process using that port and retry", llm.OpenAIAccountDefaultPort, err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d%s", port, llm.OpenAIOAuthCallbackPath)
	authorizationURL := llm.OpenAIAuthorizationURL(redirectURI, challenge, state)

	callbackCh := make(chan openAIOAuthCallback, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != llm.OpenAIOAuthCallbackPath {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		callback := openAIOAuthCallback{code: query.Get("code"), state: query.Get("state"), err: query.Get("error")}
		if callback.err != "" {
			fmt.Fprintln(w, "OpenAI login failed. You can close this window.")
		} else {
			fmt.Fprintln(w, "OpenAI login complete. You can close this window.")
		}
		callbackCh <- callback
	})}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			callbackCh <- openAIOAuthCallback{err: serveErr.Error()}
		}
	}()

	fmt.Println("OpenAI OAuth login")
	fmt.Printf("Authorization URL: %s\n", authorizationURL)
	if !loginNoBrowser {
		if err := openBrowser(authorizationURL); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not open browser: %v\n", err)
		}
	}
	fmt.Println("Waiting for the OAuth callback...")

	var callback openAIOAuthCallback
	select {
	case callback = <-callbackCh:
	case <-time.After(5 * time.Minute):
		_ = server.Shutdown(context.Background())
		return fmt.Errorf("timed out waiting for OpenAI OAuth callback")
	}
	_ = server.Shutdown(context.Background())
	if callback.err != "" {
		return fmt.Errorf("OpenAI OAuth callback failed: %s", callback.err)
	}
	if callback.state != state {
		return fmt.Errorf("OpenAI OAuth callback state did not match")
	}
	if callback.code == "" {
		return fmt.Errorf("OpenAI OAuth callback did not include an authorization code")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	credentials, err := llm.ExchangeOpenAICode(ctx, callback.code, verifier, redirectURI)
	if err != nil {
		return err
	}
	if err := llm.SaveOpenAIAccountCredentials(credentials); err != nil {
		return err
	}
	fmt.Println("OpenAI account credentials saved.")

	_, catalog, err := llm.RefreshOpenAIAccountModelCatalog(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: login succeeded but model discovery failed: %v\n", err)
		fmt.Println("Run 'ocr llm models --refresh' after checking the account connection.")
		return nil
	}
	fmt.Printf("Discovered %d OpenAI account models.\n", len(catalog.Models))
	fmt.Println("Run 'ocr config provider' to choose the model and runtime options.")
	return nil
}

func openBrowser(target string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{target}
	case "linux":
		command = "xdg-open"
		args = []string{target}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", target}
	default:
		return fmt.Errorf("unsupported operating system %q", runtime.GOOS)
	}
	return exec.Command(command, args...).Start()
}
