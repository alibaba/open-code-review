// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Package chatgpt manages local OAuth registrations for ChatGPT plan usage.
package chatgpt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofrs/flock"
	"github.com/google/uuid"
)

const (
	ProviderName    = "openai-chatgpt"
	Resource        = "https://api.openai.com/v1"
	PlanScope       = "chatgpt.tokens.use.direct"
	UsageURL        = "https://chatgpt.com/#settings/Usage"
	requestedScopes = "openid profile email offline_access resource.invoke " + PlanScope
)

type account struct {
	ClientID          string    `json:"client_id"`
	Issuer            string    `json:"issuer"`
	Subject           string    `json:"subject"`
	Email             string    `json:"email"`
	IDToken           string    `json:"id_token,omitempty"`
	AccessToken       string    `json:"access_token,omitempty"`
	RefreshToken      string    `json:"refresh_token,omitempty"`
	Scopes            []string  `json:"scopes,omitempty"`
	ExpiresAt         time.Time `json:"expires_at"`
	EarliestRefreshAt time.Time `json:"earliest_refresh_at,omitempty"`
}

type store struct {
	HostID   string              `json:"ext_agent_host_id"`
	Active   string              `json:"active_client_id,omitempty"`
	Accounts map[string]*account `json:"accounts"`
}

// Profile intentionally contains no credentials and is safe to display.
type Profile struct {
	ClientID    string
	Email       string
	Active      bool
	Connected   bool
	PlanEnabled bool
	ExpiresAt   time.Time
}

type Model struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Visibility  string `json:"visibility"`
}

type Client struct {
	path     string
	issuer   string
	resource string
	http     *http.Client
	now      func() time.Time
}

func New(path string) *Client {
	return &Client{path: path, issuer: "https://auth.openai.com", resource: Resource,
		http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, now: time.Now}
}

func DefaultClient() (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return New(filepath.Join(home, ".opencodereview", "chatgpt", "credentials.json")), nil
}

func (c *Client) load() (*store, error) {
	s := &store{Accounts: map[string]*account{}}
	data, err := os.ReadFile(c.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ChatGPT credentials: %w", err)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, errors.New("invalid ChatGPT credential file")
	}
	if s.Accounts == nil {
		s.Accounts = map[string]*account{}
	}
	return s, nil
}

func (c *Client) save(s *store) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(c.path), ".credentials-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), c.path)
}

func (c *Client) withStore(ctx context.Context, fn func(*store) error) error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	lock := flock.New(c.path+".lock", flock.SetPermissions(0o600))
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return ctx.Err()
	}
	defer lock.Close()
	s, err := c.load()
	if err != nil {
		return err
	}
	return fn(s)
}

func (c *Client) Profiles() ([]Profile, error) {
	s, err := c.load()
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(s.Accounts))
	for id, a := range s.Accounts {
		if a == nil {
			continue
		}
		profiles = append(profiles, Profile{ClientID: id, Email: a.Email, Active: id == s.Active,
			Connected: a.AccessToken != "" || a.IDToken != "", PlanEnabled: slices.Contains(a.Scopes, PlanScope), ExpiresAt: a.ExpiresAt})
	}
	slices.SortFunc(profiles, func(a, b Profile) int { return strings.Compare(a.ClientID, b.ClientID) })
	return profiles, nil
}

func (c *Client) Select(ctx context.Context, id string) error {
	return c.withStore(ctx, func(s *store) error {
		if s.Accounts[id] == nil {
			return errors.New("unknown ChatGPT account registration")
		}
		s.Active = id
		return c.save(s)
	})
}

type tokenResponse struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token"`
	IDToken           string `json:"id_token"`
	TokenType         string `json:"token_type"`
	ExpiresIn         int64  `json:"expires_in"`
	Scope             string `json:"scope"`
	EarliestRefreshAt int64  `json:"earliest_refresh_at"`
}

type oauthError struct {
	status int
	code   string
}

func (e *oauthError) Error() string {
	return fmt.Sprintf("ChatGPT OAuth request failed (HTTP %d, %s)", e.status, e.code)
}

func (c *Client) request(ctx context.Context, method, endpoint string, form url.Values, token string, dst any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ChatGPT request could not reach OpenAI: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		code := "request_rejected"
		switch e.Error {
		case "invalid_grant", "invalid_client", "access_denied":
			code = e.Error
		}
		return &oauthError{resp.StatusCode, code}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(dst); err != nil {
		return errors.New("invalid ChatGPT response")
	}
	return nil
}

func (c *Client) discovery(ctx context.Context) (*oidc.Provider, string, error) {
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, c.http), c.issuer)
	if err != nil {
		return nil, "", errors.New("could not load OpenAI identity configuration")
	}
	var metadata struct {
		RevocationEndpoint string `json:"revocation_endpoint"`
	}
	if err := p.Claims(&metadata); err != nil {
		return nil, "", err
	}
	return p, metadata.RevocationEndpoint, nil
}

func (c *Client) applyToken(a *account, t tokenResponse, refresh bool) error {
	scopes := a.Scopes
	switch {
	case t.Scope != "":
		scopes = strings.Fields(t.Scope)
	case refresh:
	case t.AccessToken != "":
		// RFC 6749 section 5.1 lets the server omit scope when it granted
		// exactly what was requested.
		scopes = strings.Fields(requestedScopes)
	default:
		scopes = nil
	}
	if !refresh && !slices.Contains(scopes, PlanScope) && t.AccessToken == "" && t.IDToken != "" {
		a.Scopes = scopes
		a.IDToken = t.IDToken
		return nil
	}
	if t.AccessToken == "" || !strings.EqualFold(t.TokenType, "Bearer") || t.ExpiresIn <= 0 || t.ExpiresIn > 86400 || (!refresh && slices.Contains(scopes, "offline_access") && t.RefreshToken == "") {
		return errors.New("incomplete ChatGPT token response")
	}
	a.Scopes = scopes
	a.AccessToken = t.AccessToken
	// RFC 6749 section 6 lets a refresh response omit refresh_token, in which
	// case the current one stays valid.
	if t.RefreshToken != "" {
		a.RefreshToken = t.RefreshToken
	}
	if t.IDToken != "" {
		a.IDToken = t.IDToken
	}
	a.ExpiresAt = c.now().Add(time.Duration(t.ExpiresIn) * time.Second)
	a.EarliestRefreshAt = time.Time{}
	if t.EarliestRefreshAt > 0 {
		a.EarliestRefreshAt = time.Unix(t.EarliestRefreshAt, 0)
	}
	return nil
}

func (c *Client) AccessToken(ctx context.Context) (string, error) {
	return c.accessToken(ctx, "")
}

// TokenSource pins a run to one registration, even if another process switches accounts.
func (c *Client) TokenSource() func(context.Context) (string, error) {
	var mu sync.Mutex
	var id string
	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if id == "" {
			s, err := c.load()
			if err != nil {
				return "", err
			}
			id = s.Active
			if id == "" {
				return "", errors.New("ChatGPT is not connected; run 'ocr llm login openai-chatgpt'")
			}
		}
		return c.accessToken(ctx, id)
	}
}

func (c *Client) accessToken(ctx context.Context, id string) (string, error) {
	var token string
	err := c.withStore(ctx, func(s *store) error {
		if id == "" {
			id = s.Active
		}
		a := s.Accounts[id]
		if a == nil {
			return errors.New("ChatGPT is not connected; run 'ocr llm login openai-chatgpt'")
		}
		if a.ClientID != id || a.Issuer != c.issuer {
			return errors.New("ChatGPT registration identity is invalid; sign in again")
		}
		if !slices.Contains(a.Scopes, PlanScope) {
			return errors.New("ChatGPT plan usage was not authorized; run 'ocr llm login openai-chatgpt'")
		}
		if a.AccessToken == "" {
			return errors.New("ChatGPT is not connected; run 'ocr llm login openai-chatgpt'")
		}
		if !c.now().Add(time.Minute).Before(a.ExpiresAt) && !c.now().Before(a.EarliestRefreshAt) {
			if a.RefreshToken == "" {
				return errors.New("ChatGPT session is not renewable; sign in again")
			}
			p, _, err := c.discovery(ctx)
			if err != nil {
				return err
			}
			var t tokenResponse
			err = c.request(ctx, http.MethodPost, p.Endpoint().TokenURL, url.Values{"grant_type": {"refresh_token"}, "client_id": {a.ClientID}, "refresh_token": {a.RefreshToken}, "resource": {c.resource}}, "", &t)
			if err != nil {
				var oe *oauthError
				if errors.As(err, &oe) && (oe.code == "invalid_grant" || oe.code == "invalid_client") {
					return fmt.Errorf("ChatGPT session is no longer renewable; run 'ocr llm login openai-chatgpt': %w", err)
				}
				return err
			}
			if err := c.applyToken(a, t, true); err != nil {
				return err
			}
			if err := c.save(s); err != nil {
				return err
			}
			if !slices.Contains(a.Scopes, PlanScope) {
				return errors.New("ChatGPT plan permission is no longer granted; sign in again")
			}
		}
		if !c.now().Before(a.ExpiresAt) {
			return errors.New("ChatGPT access token has expired; retry after the permitted refresh time")
		}
		token = a.AccessToken
		return nil
	})
	return token, err
}

func (c *Client) Models(ctx context.Context) ([]Model, error) {
	token, err := c.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	var result struct {
		Models []Model `json:"models"`
	}
	if err := c.request(ctx, http.MethodGet, c.resource+"/models", nil, token, &result); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(result.Models))
	for _, m := range result.Models {
		if m.Visibility == "list" && m.Slug != "" {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("no models are available for this ChatGPT account")
	}
	return models, nil
}

func (c *Client) Logout(ctx context.Context) error {
	return c.withStore(ctx, func(s *store) error {
		a := s.Accounts[s.Active]
		if a == nil {
			return nil
		}
		var remoteErr error
		// Without offline_access there is no refresh token, but the access token
		// stays usable until expiry unless it is revoked too.
		token, hint := a.RefreshToken, "refresh_token"
		if token == "" {
			token, hint = a.AccessToken, "access_token"
		}
		if token != "" {
			var revoke string
			_, revoke, remoteErr = c.discovery(ctx)
			if remoteErr == nil && revoke == "" {
				remoteErr = errors.New("OpenAI did not publish a revocation endpoint")
			}
			if remoteErr == nil {
				for attempt := 0; attempt < 3; attempt++ {
					remoteErr = c.request(ctx, http.MethodPost, revoke, url.Values{"token": {token}, "token_type_hint": {hint}, "client_id": {a.ClientID}}, "", nil)
					var oe *oauthError
					if remoteErr == nil || (errors.As(remoteErr, &oe) && oe.status < 500) || ctx.Err() != nil || attempt == 2 {
						break
					}
					timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
					select {
					case <-ctx.Done():
					case <-timer.C:
					}
					timer.Stop()
				}
			}
		}
		a.AccessToken = ""
		a.RefreshToken = ""
		a.IDToken = ""
		a.Scopes = nil
		a.ExpiresAt = time.Time{}
		a.EarliestRefreshAt = time.Time{}
		if err := c.save(s); err != nil {
			return err
		}
		if remoteErr != nil {
			return fmt.Errorf("signed out locally; remote revocation was not confirmed; disconnect the app in ChatGPT Settings: %w", remoteErr)
		}
		return nil
	})
}

func (c *Client) prepare(ctx context.Context, id string) (string, *account, error) {
	var host string
	var selected *account
	err := c.withStore(ctx, func(s *store) error {
		if s.HostID == "" {
			s.HostID = "urn:uuid:" + uuid.NewString()
			if err := c.save(s); err != nil {
				return err
			}
		}
		host = s.HostID
		if id != "" {
			if s.Accounts[id] == nil {
				return errors.New("unknown ChatGPT account registration")
			}
			copy := *s.Accounts[id]
			selected = &copy
		}
		return nil
	})
	return host, selected, err
}
