// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

func randomValue() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// Login keeps an existing active account intact until the new identity is verified.
// An empty client ID creates a new account/workspace registration.
func (c *Client) Login(ctx context.Context, clientID string, open func(string) error) (Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	host, selected, err := c.prepare(ctx, clientID)
	if err != nil {
		return Profile{}, err
	}
	p, _, err := c.discovery(ctx)
	if err != nil {
		return Profile{}, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return Profile{}, fmt.Errorf("start ChatGPT callback listener: %w", err)
	}
	redirect := "http://" + listener.Addr().String() + "/auth/callback"
	state, nonce, verifier := randomValue(), randomValue(), randomValue()
	challenge := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "redirect_uri": {redirect}, "scope": {requestedScopes}, "resource": {c.resource}, "state": {state}, "nonce": {nonce}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "ext_agent_host_id": {host}}
	if selected == nil {
		q.Set("client_id", "dynamic_agent_client")
		q.Set("agent_name_hint", "open-code-review")
	} else {
		q.Set("client_id", selected.ClientID)
		if !slices.Contains(selected.Scopes, PlanScope) {
			q.Set("prompt", "consent")
		}
	}
	u, err := url.Parse(p.Endpoint().AuthURL)
	if err != nil {
		listener.Close()
		return Profile{}, err
	}
	u.RawQuery = q.Encode()
	type callback struct {
		code, id string
		err      error
	}
	returned := make(chan callback, 1)
	var once sync.Once
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path != "/auth/callback" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		params, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(params["state"]) != 1 || subtle.ConstantTimeCompare([]byte(params.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid sign-in state.", http.StatusBadRequest)
			return
		}
		result := callback{code: params.Get("code"), id: params.Get("client_id")}
		if params.Get("error") != "" {
			result.err = errors.New("ChatGPT sign-in was declined or failed")
		}
		if result.err == nil && (len(params["code"]) != 1 || result.code == "" || len(params["client_id"]) > 1) {
			result.err = errors.New("incomplete ChatGPT callback")
		}
		if selected != nil {
			if result.id != "" && result.id != selected.ClientID {
				result.err = errors.New("ChatGPT callback changed the selected client ID")
			}
			result.id = selected.ClientID
		} else if !strings.HasPrefix(result.id, "oaiapp_") {
			result.err = errors.New("ChatGPT did not issue a client ID")
		}
		accepted := false
		once.Do(func() { returned <- result; accepted = true })
		if !accepted {
			http.Error(w, "Sign-in callback was already used.", http.StatusConflict)
			return
		}
		if result.err != nil {
			http.Error(w, "Sign-in failed. Return to your terminal.", http.StatusBadRequest)
			return
		}
		_, _ = fmt.Fprintln(w, "Sign-in received. Return to your terminal to finish connecting ChatGPT.")
	})
	go func() { _ = server.Serve(listener) }()
	// Shutdown rather than Close: an early return on a failed callback must not
	// cut off the page telling the browser user what happened.
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelShutdown()
		if server.Shutdown(shutdownCtx) != nil {
			_ = server.Close()
		}
	}()
	if err := open(u.String()); err != nil {
		return Profile{}, err
	}
	var result callback
	select {
	case result = <-returned:
	case <-ctx.Done():
		return Profile{}, ctx.Err()
	}
	if result.err != nil {
		return Profile{}, result.err
	}
	var t tokenResponse
	if err := c.request(ctx, http.MethodPost, p.Endpoint().TokenURL, url.Values{"grant_type": {"authorization_code"}, "client_id": {result.id}, "code": {result.code}, "code_verifier": {verifier}, "redirect_uri": {redirect}, "resource": {c.resource}}, "", &t); err != nil {
		return Profile{}, err
	}
	idToken, err := p.VerifierContext(oidc.ClientContext(ctx, c.http), &oidc.Config{ClientID: result.id}).Verify(ctx, t.IDToken)
	if err != nil {
		return Profile{}, errors.New("OpenAI ID token validation failed")
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonce)) != 1 || idToken.Subject == "" {
		return Profile{}, errors.New("OpenAI ID token nonce or subject did not match")
	}
	if selected != nil && idToken.Subject != selected.Subject {
		return Profile{}, errors.New("ChatGPT sign-in returned a different account")
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Profile{}, err
	}
	a := &account{ClientID: result.id, Issuer: c.issuer, Subject: idToken.Subject, Email: claims.Email}
	if err := c.applyToken(a, t, false); err != nil {
		return Profile{}, err
	}
	active := false
	err = c.withStore(ctx, func(s *store) error {
		existing := s.Accounts[a.ClientID]
		if existing != nil && existing.Subject != a.Subject {
			return errors.New("ChatGPT registration identity changed")
		}
		if existing != nil && slices.Contains(existing.Scopes, PlanScope) && !slices.Contains(a.Scopes, PlanScope) {
			return errors.New("ChatGPT sign-in did not grant plan usage; the existing connection was kept")
		}
		// A grant without plan permission cannot run inference, so it must not
		// displace an active registration that still can.
		current := s.Accounts[s.Active]
		if slices.Contains(a.Scopes, PlanScope) || current == nil || !slices.Contains(current.Scopes, PlanScope) {
			s.Active = a.ClientID
		}
		active = s.Active == a.ClientID
		s.Accounts[a.ClientID] = a
		return c.save(s)
	})
	if err != nil {
		return Profile{}, err
	}
	return Profile{ClientID: a.ClientID, Email: a.Email, Active: active, Connected: true, PlanEnabled: slices.Contains(a.Scopes, PlanScope), ExpiresAt: a.ExpiresAt}, nil
}
