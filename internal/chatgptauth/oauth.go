// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

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
	"strings"
	"sync"
	"time"
)

const Issuer = "https://auth.openai.com"
const Resource = "https://api.openai.com/v1"
const DynamicClient = "dynamic_agent_client"
const Scope = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
const UsageURL = "https://chatgpt.com/settings/usage"

type Client struct {
	base string
	http *http.Client
	keys keyCache
}

func NewClient() *Client {
	return &Client{base: Issuer, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

var openBrowser = openBrowserURL

type discovery struct {
	Issuer     string `json:"issuer"`
	JWKS       string `json:"jwks_uri"`
	Revocation string `json:"revocation_endpoint"`
}

func (c *Client) safeEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	b, baseErr := url.Parse(c.base)
	return err == nil && baseErr == nil && u.Scheme == b.Scheme && u.Host == b.Host && u.User == nil && u.Fragment == "" && u.RawQuery == ""
}
func (c *Client) discover(ctx context.Context, d *discovery) error {
	if err := c.get(ctx, c.base+"/.well-known/openid-configuration", d); err != nil {
		return err
	}
	if d.Issuer != Issuer || !c.safeEndpoint(d.JWKS) || !c.safeEndpoint(d.Revocation) {
		return errors.New("OpenAI discovery endpoints or issuer did not match")
	}
	return nil
}
func (c *Client) get(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	return c.request(req, target)
}

type OAuthError struct {
	Status    int
	Code      string
	RequestID string
}

func (e *OAuthError) Error() string {
	return fmt.Sprintf("ChatGPT OAuth request failed: status=%d code=%s request_id=%s", e.Status, e.Code, e.RequestID)
}
func knownCode(code string) string {
	switch code {
	case "invalid_request", "invalid_client", "invalid_grant", "invalid_scope", "access_denied", "unauthorized_client", "unsupported_grant_type", "server_error", "temporarily_unavailable", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused":
		return code
	}
	return "unknown_error"
}
func (c *Client) request(req *http.Request, target any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ChatGPT OAuth transport failed: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("ChatGPT OAuth response exceeded size limit")
	}
	if resp.StatusCode != 200 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		return &OAuthError{Status: resp.StatusCode, Code: knownCode(e.Error), RequestID: resp.Header.Get("x-request-id")}
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(b, target); err != nil {
		return errors.New("invalid ChatGPT OAuth response JSON")
	}
	return nil
}
func (c *Client) post(ctx context.Context, endpoint string, form url.Values, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.request(req, target)
}
func randomValue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type attempt struct {
	state, nonce, verifier, redirect, host string
	consent                                bool
}

func (c *Client) authorize(a attempt, old *Auth) string {
	clientID := DynamicClient
	if old != nil {
		clientID = old.ClientID
	}
	sum := sha256.Sum256([]byte(a.verifier))
	q := url.Values{"client_id": {clientID}, "ext_agent_host_id": {a.host}, "response_type": {"code"}, "redirect_uri": {a.redirect}, "scope": {Scope}, "resource": {Resource}, "state": {a.state}, "nonce": {a.nonce}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}}
	if a.consent {
		q.Set("prompt", "consent")
	}
	if old == nil {
		q.Set("agent_name_hint", "OpenCodeReview")
	} else {
		if old.IDToken != "" {
			q.Set("id_token_hint", old.IDToken)
		}
		if old.Email != "" {
			q.Set("login_hint", old.Email)
		}
	}
	return c.base + "/api/accounts/authorize?" + q.Encode()
}
func (a attempt) callback(q url.Values, old *Auth) (string, string, error) {
	for _, key := range []string{"state", "code", "client_id", "error"} {
		if len(q[key]) > 1 {
			return "", "", errors.New("duplicate OAuth callback parameter")
		}
	}
	if q.Get("state") != a.state || a.state == "" {
		return "", "", errors.New("OAuth callback state did not match")
	}
	if q.Get("error") != "" {
		return "", "", errors.New("ChatGPT authorization was declined")
	}
	code := q.Get("code")
	if code == "" {
		return "", "", errors.New("OAuth callback did not include an authorization code")
	}
	id := q.Get("client_id")
	if old != nil {
		if id != "" && id != old.ClientID {
			return "", "", errors.New("OAuth callback changed the selected client ID")
		}
		id = old.ClientID
	}
	if id == "" || id == DynamicClient {
		return "", "", errors.New("OAuth registration did not issue a client ID")
	}
	return code, id, nil
}

type tokenResponse struct {
	Access   string          `json:"access_token"`
	Refresh  string          `json:"refresh_token"`
	ID       string          `json:"id_token"`
	Type     string          `json:"token_type"`
	Expires  int64           `json:"expires_in"`
	Scope    *string         `json:"scope"`
	Earliest json.RawMessage `json:"earliest_refresh_at"`
}

func (t tokenResponse) apply(a *Auth, refresh bool) error {
	if t.Access == "" || t.Refresh == "" || !strings.EqualFold(t.Type, "Bearer") || t.Expires <= 0 || t.Expires > 86400 {
		return errors.New("ChatGPT token response is missing credentials, Bearer type or valid expiry")
	}
	if !refresh && (t.ID == "" || t.Scope == nil) {
		return errors.New("ChatGPT sign-in response is missing ID token or granted scopes")
	}
	now := time.Now()
	earliest := time.Time{}
	if len(t.Earliest) > 0 && string(t.Earliest) != "null" {
		var seconds int64
		if json.Unmarshal(t.Earliest, &seconds) == nil {
			earliest = time.Unix(seconds, 0)
		} else {
			var text string
			if json.Unmarshal(t.Earliest, &text) != nil {
				return errors.New("invalid earliest_refresh_at")
			}
			var err error
			earliest, err = time.Parse(time.RFC3339, text)
			if err != nil {
				return errors.New("invalid earliest_refresh_at")
			}
		}
		if earliest.After(now.Add(time.Duration(t.Expires) * time.Second)) {
			return errors.New("earliest_refresh_at exceeds token expiry")
		}
	}
	a.AccessToken = t.Access
	a.RefreshToken = t.Refresh
	a.TokenType = "Bearer"
	a.ExpiresAt = now.Add(time.Duration(t.Expires) * time.Second)
	a.EarliestRefreshAt = earliest
	if t.ID != "" {
		a.IDToken = t.ID
	}
	if t.Scope != nil {
		a.Scopes = strings.Fields(*t.Scope)
	}
	return nil
}
func (c *Client) exchange(ctx context.Context, a attempt, code, id string, old *Auth) (*Auth, error) {
	var t tokenResponse
	if err := c.post(ctx, c.base+"/api/accounts/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {id}, "code": {code}, "code_verifier": {a.verifier}, "redirect_uri": {a.redirect}, "resource": {Resource}}, &t); err != nil {
		return nil, err
	}
	identity, err := c.verify(ctx, t.ID, id, a.nonce)
	if err != nil {
		return nil, err
	}
	if old != nil && (identity.Subject != old.Subject || identity.Issuer != old.Issuer) {
		return nil, errors.New("ChatGPT sign-in identity did not match the selected registration")
	}
	auth := &Auth{ClientID: id, Issuer: identity.Issuer, Subject: identity.Subject, Email: identity.Email, Nonce: a.nonce}
	if old != nil {
		auth.Welcomed = old.Welcomed
	}
	if err := t.apply(auth, false); err != nil {
		return nil, err
	}
	return auth, nil
}

func (c *Client) Login(ctx context.Context, s Store, account string, newAccount, noBrowser, consent bool, out io.Writer) (*Auth, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if consent && newAccount {
		return nil, errors.New("--enable-plan requires a retained ChatGPT registration")
	}
	host, err := s.Host(ctx)
	if err != nil {
		return nil, err
	}
	var old *Auth
	err = s.withLock(ctx, func() error {
		d, err := s.load()
		if err != nil {
			return err
		}
		if !newAccount && (len(d.Accounts) > 0 || consent || account != "") {
			selected, err := d.selected(account)
			if err != nil {
				return err
			}
			copy := *selected
			old = &copy
		} else if account != "" {
			return errors.New("an account cannot be selected for new registration")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	registration := old
	var auth *Auth
	for n := 0; n < 2; n++ {
		var issued string
		auth, err = c.loginAttempt(ctx, host, registration, old, noBrowser, consent, out, &issued)
		var oauth *OAuthError
		if n == 0 && issued != "" && errors.As(err, &oauth) && oauth.Code == "invalid_grant" {
			// Keep the issued client only within this pending authorization, never as an active identity.
			registration = &Auth{ClientID: issued}
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}
	err = s.withLock(ctx, func() error {
		d, err := s.load()
		if err != nil {
			return err
		}
		if old != nil {
			current, err := d.selected(old.ClientID)
			if err != nil {
				return err
			}
			if current.Revision != old.Revision {
				return errors.New("ChatGPT registration changed during sign-in; start sign-in again")
			}
			if current.Subject != auth.Subject || current.Issuer != auth.Issuer {
				return errors.New("issued registration conflicts with saved identity")
			}
			auth.Revision = current.Revision + 1
			*current = *auth
		} else {
			for _, saved := range d.Accounts {
				if saved.ClientID == auth.ClientID {
					return errors.New("issued registration already exists; sign in using that account")
				}
			}
			auth.Revision = 1
			d.Accounts = append(d.Accounts, *auth)
		}
		d.Active = auth.ClientID
		if auth.PlanEnabled() && !auth.Welcomed {
			if _, err := fmt.Fprintln(out, "You're using your ChatGPT plan. Manage usage:", UsageURL); err != nil {
				return err
			}
			auth.Welcomed = true
			for i := range d.Accounts {
				if d.Accounts[i].ClientID == auth.ClientID {
					d.Accounts[i].Welcomed = true
				}
			}
		}
		return s.save(d)
	})
	if err != nil {
		return nil, err
	}
	return auth, nil
}

func (c *Client) loginAttempt(ctx context.Context, host string, registration, old *Auth, noBrowser, consent bool, out io.Writer, issued *string) (*Auth, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	a := attempt{consent: consent, host: host, redirect: "http://" + listener.Addr().String() + "/auth/callback"}
	if a.state, err = randomValue(); err != nil {
		return nil, err
	}
	if a.nonce, err = randomValue(); err != nil {
		return nil, err
	}
	if a.verifier, err = randomValue(); err != nil {
		return nil, err
	}
	type result struct {
		code, id string
		err      error
	}
	done := make(chan result, 1)
	var once sync.Once
	startOnce := sync.Once{}
	mux := http.NewServeMux()
	target := c.authorize(a, registration)
	mux.HandleFunc("/auth/start/"+a.state, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Host != listener.Addr().String() {
			http.Error(w, "invalid callback origin", 403)
			return
		}
		served := false
		startOnce.Do(func() {
			served = true
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, r, target, http.StatusFound)
		})
		if !served {
			http.Error(w, "sign-in already started", 410)
		}
	})
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Host != listener.Addr().String() {
			http.Error(w, "invalid callback origin", 403)
			return
		}
		q, parseErr := url.ParseQuery(r.URL.RawQuery)
		code, id, callbackErr := a.callback(q, registration)
		if parseErr != nil {
			callbackErr = errors.New("invalid OAuth callback query")
		}
		consumed := false
		once.Do(func() { consumed = true; done <- result{code: code, id: id, err: callbackErr} })
		w.Header().Set("Cache-Control", "no-store")
		if !consumed {
			http.Error(w, "callback already consumed", 410)
		} else if callbackErr != nil {
			http.Error(w, "Sign-in could not be verified.", 400)
		} else {
			fmt.Fprintln(w, "Authorization received. Return to OpenCodeReview to confirm sign-in.")
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	defer server.Close()
	localURL := "http://" + listener.Addr().String() + "/auth/start/" + a.state
	if _, err := fmt.Fprintln(out, "Continue with ChatGPT. Eligible requests use your ChatGPT plan."); err != nil {
		return nil, err
	}
	if noBrowser {
		if _, err := fmt.Fprintln(out, "Open this local URL in your browser:", localURL); err != nil {
			return nil, err
		}
	} else if err := openBrowser(ctx, localURL); err != nil {
		return nil, errors.New("could not open browser; retry with --no-browser")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-serveErr:
		return nil, fmt.Errorf("ChatGPT callback server stopped: %w", err)
	case r := <-done:
		if r.err != nil {
			return nil, r.err
		}
		*issued = r.id
		auth, err := c.exchange(ctx, a, r.code, r.id, old)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return auth, nil
	}
}

func terminalRefresh(err error) bool {
	var e *OAuthError
	if !errors.As(err, &e) {
		return false
	}
	switch e.Code {
	case "invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused":
		return true
	}
	return false
}
func (c *Client) Credentials(ctx context.Context, s Store, account string) (*Auth, error) {
	var result *Auth
	err := s.withLock(ctx, func() error {
		d, err := s.load()
		if err != nil {
			return err
		}
		auth, err := d.selected(account)
		if err != nil {
			return err
		}
		if auth.AccessToken == "" {
			return errors.New("ChatGPT account is signed out; run ocr auth login --provider chatgpt")
		}
		if !auth.PlanEnabled() {
			return fmt.Errorf("ChatGPT plan usage is disabled; run ocr auth login --provider chatgpt --enable-plan --account %s or select an API-key provider", auth.ClientID)
		}
		now := time.Now()
		if auth.ExpiresAt.After(now.Add(5*time.Minute)) || (!auth.EarliestRefreshAt.IsZero() && now.Before(auth.EarliestRefreshAt) && now.Before(auth.ExpiresAt)) {
			copy := *auth
			result = &copy
			return nil
		}
		if auth.RefreshToken == "" {
			return errors.New("ChatGPT refresh token is missing; sign in again")
		}
		var t tokenResponse
		err = c.post(ctx, c.base+"/api/accounts/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {auth.ClientID}, "refresh_token": {auth.RefreshToken}, "resource": {Resource}}, &t)
		if err != nil {
			if terminalRefresh(err) {
				auth.clear()
				if saveErr := s.save(d); saveErr != nil {
					return saveErr
				}
			}
			return err
		}
		rotated := *auth
		if t.ID != "" {
			identity, err := c.verify(ctx, t.ID, auth.ClientID, "")
			if err != nil {
				return err
			}
			if identity.Subject != auth.Subject || identity.Issuer != auth.Issuer {
				return errors.New("refresh changed ChatGPT identity")
			}
			if identity.Nonce != "" && identity.Nonce != auth.Nonce {
				return errors.New("refresh changed the original ID-token nonce")
			}
			rotated.Email = identity.Email
		}
		if err := t.apply(&rotated, true); err != nil {
			return err
		}
		rotated.Revision++
		*auth = rotated
		if err := s.save(d); err != nil {
			return err
		}
		// A validated replacement must survive cancellation after the server rotated the token.
		if err := ctx.Err(); err != nil {
			return err
		}
		if !rotated.PlanEnabled() {
			return fmt.Errorf("refreshed ChatGPT grant does not permit plan usage; run ocr auth login --provider chatgpt --enable-plan --account %s", auth.ClientID)
		}
		result = &rotated
		return nil
	})
	return result, err
}

func (c *Client) revokeSession(ctx context.Context, auth *Auth) error {
	var err error
	for n := 0; n < 3; n++ {
		var config discovery
		err = c.discover(ctx, &config)
		if err == nil {
			err = c.post(ctx, config.Revocation, url.Values{"token": {auth.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {auth.ClientID}}, nil)
		}
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var oe *OAuthError
		var networkErr *url.Error
		if !(errors.As(err, &oe) && oe.Status >= 500) && !errors.As(err, &networkErr) {
			return err
		}
		if n < 2 {
			timer := time.NewTimer(time.Duration(n+1) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return err
}

func (c *Client) Logout(ctx context.Context, s Store, account string) error {
	return s.withLock(ctx, func() error {
		d, err := s.load()
		if err != nil {
			return err
		}
		a, err := d.selected(account)
		if err != nil {
			return err
		}
		var revokeErr error
		if a.RefreshToken != "" {
			revokeErr = c.revokeSession(ctx, a)
		}
		a.clear()
		if err := s.save(d); err != nil {
			return err
		}
		if revokeErr != nil {
			return fmt.Errorf("local tokens cleared; remote revocation not confirmed, disconnect the app in ChatGPT Settings: %w", revokeErr)
		}
		return nil
	})
}
