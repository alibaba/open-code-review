// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package codexauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	Issuer      = "https://auth.openai.com"
	ClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	RedirectURI = "http://localhost:1455/auth/callback"
	Scope       = "openid profile email offline_access api.connectors.read api.connectors.invoke"

	refreshBefore         = 5 * time.Minute
	lockRetry             = 50 * time.Millisecond
	refreshLockStaleAfter = time.Minute
	oauthHTTPTimeout      = 30 * time.Second
	maxOAuthTokenLifetime = 365 * 24 * time.Hour
)

// NeedsRefresh reports whether the access token should be refreshed before use.
func (a *CodexAuth) NeedsRefresh(now time.Time) bool {
	return a == nil || a.ExpiresAt.IsZero() || !a.ExpiresAt.After(now.Add(refreshBefore))
}

var openBrowser = openBrowserURL

// OAuthClient performs Codex OAuth requests. Issuer and HTTPClient are
// configurable to keep the network protocol testable without live services.
type OAuthClient struct {
	Issuer     string
	HTTPClient *http.Client
}

type pkceCodes struct {
	verifier  string
	challenge string
}

type callbackResult struct {
	code string
	err  error
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

// NewOAuthClient returns a client configured for OpenAI's production issuer.
func NewOAuthClient() *OAuthClient {
	return &OAuthClient{Issuer: Issuer, HTTPClient: &http.Client{Timeout: oauthHTTPTimeout}}
}

func (c *OAuthClient) issuer() string {
	if c == nil || c.Issuer == "" {
		return Issuer
	}
	return strings.TrimRight(c.Issuer, "/")
}

func (c *OAuthClient) httpClient() *http.Client {
	if c == nil || c.HTTPClient == nil {
		return http.DefaultClient
	}
	return c.HTTPClient
}

func newPKCE() (pkceCodes, error) {
	verifier, err := randomURLSafe(32)
	if err != nil {
		return pkceCodes{}, err
	}
	digest := sha256.Sum256([]byte(verifier))
	return pkceCodes{
		verifier:  verifier,
		challenge: base64.RawURLEncoding.EncodeToString(digest[:]),
	}, nil
}

func randomURLSafe(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate OAuth random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (c *OAuthClient) authorizeURL(pkce pkceCodes, state string) string {
	values := url.Values{
		"response_type":              {"code"},
		"client_id":                  {ClientID},
		"redirect_uri":               {RedirectURI},
		"scope":                      {Scope},
		"code_challenge":             {pkce.challenge},
		"code_challenge_method":      {"S256"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"state":                      {state},
		"originator":                 {"codex_cli_rs"},
	}
	return c.issuer() + "/oauth/authorize?" + values.Encode()
}

// LoginLoopback completes browser-based PKCE authentication and saves the
// resulting credentials. When noBrowser is true, it only prints the URL.
func (c *OAuthClient) LoginLoopback(ctx context.Context, store CodexStore, noBrowser bool, out io.Writer) (*CodexAuth, error) {
	if store == nil {
		return nil, errors.New("Codex credential store is required")
	}
	if out == nil {
		out = io.Discard
	}
	pkce, err := newPKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomURLSafe(32)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", "localhost:1455")
	if err != nil {
		return nil, fmt.Errorf("start OAuth callback listener on localhost:1455: %w", err)
	}
	result := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", callbackHandler(state, result))
	server := &http.Server{Handler: mux}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	authorizeURL := c.authorizeURL(pkce, state)
	if noBrowser {
		_, _ = fmt.Fprintf(out, "Open this URL to sign in:\n%s\n", authorizeURL)
	} else if err := openBrowser(authorizeURL); err != nil {
		_, _ = fmt.Fprintf(out, "Could not open a browser. Open this URL to sign in:\n%s\n", authorizeURL)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return nil, fmt.Errorf("serve OAuth callback: %w", err)
		}
		return nil, errors.New("OAuth callback server closed before sign-in completed")
	case callback := <-result:
		if callback.err != nil {
			return nil, callback.err
		}
		auth, err := c.exchangeCode(ctx, callback.code, RedirectURI, pkce.verifier, time.Now())
		if err != nil {
			return nil, err
		}
		if err := store.Save(auth); err != nil {
			return nil, fmt.Errorf("save Codex credentials: %w", err)
		}
		return auth, nil
	}
}

func callbackHandler(expectedState string, result chan<- callbackResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		query := r.URL.Query()
		if query.Get("state") != expectedState {
			http.Error(w, "invalid OAuth state", http.StatusBadRequest)
			return
		}
		if query.Get("error") != "" {
			http.Error(w, "authorization was denied", http.StatusBadRequest)
			sendCallbackResult(result, callbackResult{err: errors.New("Codex authorization was denied")})
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			sendCallbackResult(result, callbackResult{err: errors.New("OAuth callback did not contain an authorization code")})
			return
		}
		_, _ = io.WriteString(w, "Sign-in complete. You can close this window.\n")
		sendCallbackResult(result, callbackResult{code: code})
	}
}

func sendCallbackResult(result chan<- callbackResult, value callbackResult) {
	select {
	case result <- value:
	default:
	}
}

func isLoopbackHost(hostPort string) bool {
	host := hostPort
	if parsed, _, err := net.SplitHostPort(hostPort); err == nil {
		host = parsed
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func (c *OAuthClient) exchangeCode(ctx context.Context, code, redirectURI, verifier string, now time.Time) (*CodexAuth, error) {
	values := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {ClientID},
		"code_verifier": {verifier},
	}
	var response tokenResponse
	if err := c.postForm(ctx, c.issuer()+"/oauth/token", values, &response); err != nil {
		return nil, fmt.Errorf("exchange OAuth authorization code: %w", err)
	}
	return authFromTokenResponse(response, now)
}

func (c *OAuthClient) postForm(ctx context.Context, endpoint string, values url.Values, target any) error {
	return c.post(ctx, endpoint, "application/x-www-form-urlencoded", strings.NewReader(values.Encode()), target)
}

func (c *OAuthClient) postJSON(ctx context.Context, endpoint string, body any, target any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode OAuth request: %w", err)
	}
	return c.post(ctx, endpoint, "application/json", strings.NewReader(string(encoded)), target)
}

func (c *OAuthClient) post(ctx context.Context, endpoint, contentType string, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("create OAuth request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	response, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("send OAuth request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("OAuth endpoint returned %s", response.Status)
	}
	if target == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode OAuth response: %w", err)
	}
	return nil
}

func authFromTokenResponse(response tokenResponse, now time.Time) (*CodexAuth, error) {
	if response.AccessToken == "" || response.RefreshToken == "" {
		return nil, errors.New("OAuth response did not contain access and refresh tokens")
	}
	if response.ExpiresIn <= 0 {
		return nil, errors.New("OAuth response did not contain a positive token lifetime")
	}
	if response.ExpiresIn > int64(maxOAuthTokenLifetime/time.Second) {
		return nil, errors.New("OAuth response token lifetime exceeds the supported maximum")
	}
	accountID, planType := tokenMetadata(response.IDToken, response.AccessToken)
	return &CodexAuth{
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
		IDToken:      response.IDToken,
		AccountID:    accountID,
		PlanType:     planType,
		ExpiresAt:    now.Add(time.Duration(response.ExpiresIn) * time.Second),
	}, nil
}

func tokenMetadata(tokens ...string) (string, string) {
	for _, token := range tokens {
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			continue
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		var claims struct {
			Auth struct {
				AccountID string `json:"chatgpt_account_id"`
				PlanType  string `json:"chatgpt_plan_type"`
			} `json:"https://api.openai.com/auth"`
		}
		if json.Unmarshal(payload, &claims) == nil && (claims.Auth.AccountID != "" || claims.Auth.PlanType != "") {
			return claims.Auth.AccountID, claims.Auth.PlanType
		}
	}
	return "", ""
}

// RefreshIfNeeded reloads credentials under a cross-process lock and refreshes
// tokens close to expiry. The rotated refresh token is saved before the new
// access token is returned to the caller.
func (c *OAuthClient) RefreshIfNeeded(ctx context.Context, store CodexStore, now func() time.Time) (*CodexAuth, error) {
	if store == nil {
		return nil, errors.New("Codex credential store is required")
	}
	if now == nil {
		now = time.Now
	}
	auth, err := store.Load()
	if err != nil {
		return nil, err
	}
	if !auth.NeedsRefresh(now()) {
		return auth, nil
	}

	release, err := acquireRefreshLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	auth, err = store.Load()
	if err != nil {
		return nil, err
	}
	if !auth.NeedsRefresh(now()) {
		return auth, nil
	}
	if auth.RefreshToken == "" {
		return nil, errors.New("Codex credentials do not contain a refresh token; run: ocr auth login")
	}

	request := struct {
		ClientID     string `json:"client_id"`
		GrantType    string `json:"grant_type"`
		RefreshToken string `json:"refresh_token"`
	}{
		ClientID:     ClientID,
		GrantType:    "refresh_token",
		RefreshToken: auth.RefreshToken,
	}
	var response tokenResponse
	if err := c.postJSON(ctx, c.issuer()+"/oauth/token", request, &response); err != nil {
		return nil, fmt.Errorf("refresh Codex credentials: %w", err)
	}
	if response.RefreshToken == "" {
		response.RefreshToken = auth.RefreshToken
	}
	rotated, err := authFromTokenResponse(response, now())
	if err != nil {
		return nil, fmt.Errorf("refresh Codex credentials: %w", err)
	}
	if err := store.Save(rotated); err != nil {
		return nil, fmt.Errorf("save refreshed Codex credentials: %w", err)
	}
	return rotated, nil
}

func acquireRefreshLock(ctx context.Context) (func(), error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create Codex auth directory: %w", err)
	}
	lockPath := path + ".lock"
	for {
		file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, writeErr := fmt.Fprintf(file, "%d\n", os.Getpid()); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("write Codex refresh lock owner: %w", writeErr)
			}
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(lockPath)
				return nil, fmt.Errorf("close Codex refresh lock: %w", closeErr)
			}
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create Codex refresh lock: %w", err)
		}
		info, statErr := os.Stat(lockPath)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect Codex refresh lock: %w", statErr)
		}
		if statErr == nil && time.Since(info.ModTime()) > refreshLockStaleAfter {
			ownerAlive, ownerErr := refreshLockOwnerAlive(lockPath)
			if ownerErr != nil {
				return nil, ownerErr
			}
			// Age alone is not proof that a configurable refresh request has stopped.
			if !ownerAlive {
				if removeErr := os.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					return nil, fmt.Errorf("remove stale Codex refresh lock: %w", removeErr)
				}
				continue
			}
		}
		timer := time.NewTimer(lockRetry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("wait for Codex credential refresh: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func refreshLockOwnerAlive(lockPath string) (bool, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read Codex refresh lock owner: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false, fmt.Errorf(
			"stale Codex refresh lock %q has no valid owner PID; remove it after confirming no OCR process is refreshing credentials",
			lockPath,
		)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return errors.Is(err, os.ErrPermission), nil
	}
	defer process.Release()
	if runtime.GOOS == "windows" {
		// FindProcess uses OpenProcess on Windows, so a successful lookup proves the owner still exists.
		return true, nil
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission), nil
}

// Revoke asks OpenAI to revoke the refresh token.
func (c *OAuthClient) Revoke(ctx context.Context, auth *CodexAuth) error {
	if auth == nil || auth.RefreshToken == "" {
		return errors.New("Codex credentials do not contain a refresh token")
	}
	values := url.Values{
		"token":           {auth.RefreshToken},
		"token_type_hint": {"refresh_token"},
		"client_id":       {ClientID},
	}
	return c.postForm(ctx, c.issuer()+"/oauth/revoke", values, nil)
}
