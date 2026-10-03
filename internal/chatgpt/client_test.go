// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type identityFixture struct {
	client         *Client
	server         *httptest.Server
	signer         jose.Signer
	key            *rsa.PrivateKey
	mu             sync.Mutex
	authorization  url.Values
	scope          string
	subject        string
	audience       string
	nonceOverride  string
	issuerOverride string
	expired        bool
	tokenStatus    int
	revokeStatus   int
	identityOnly   bool
	noRotation     bool
	refreshCount   atomic.Int32
	revokeCount    atomic.Int32
	revokedHint    atomic.Value
}

func newIdentityFixture(t *testing.T) *identityFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
	if err != nil {
		t.Fatal(err)
	}
	f := &identityFixture{key: key, signer: signer, scope: requestedScopes, subject: "user-1", audience: "oaiapp_test"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": f.server.URL, "authorization_endpoint": f.server.URL + "/authorize", "token_endpoint": f.server.URL + "/token", "jwks_uri": f.server.URL + "/jwks", "revocation_endpoint": f.server.URL + "/revoke", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if r.Form.Get("client_id") != "oaiapp_test" || r.Form.Get("resource") != f.server.URL+"/v1" || r.Form.Get("client_secret") != "" {
				t.Error("invalid token exchange parameters")
			}
			if f.tokenStatus != 0 {
				w.WriteHeader(f.tokenStatus)
				_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
				return
			}
			if r.Form.Get("grant_type") == "refresh_token" {
				f.refreshCount.Add(1)
				if r.Form.Has("scope") {
					t.Error("refresh must retain the existing grant")
				}
				response := tokenResponse{AccessToken: "refreshed-access", RefreshToken: "rotated-refresh", TokenType: "Bearer", ExpiresIn: 3600}
				if f.noRotation {
					response.RefreshToken = ""
				}
				_ = json.NewEncoder(w).Encode(response)
				return
			}
			f.mu.Lock()
			authorization := f.authorization
			f.mu.Unlock()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if authorization.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(hash[:]) || authorization.Get("redirect_uri") != r.Form.Get("redirect_uri") {
				t.Error("PKCE or redirect mismatch")
			}
			nonce := authorization.Get("nonce")
			if f.nonceOverride != "" {
				nonce = f.nonceOverride
			}
			exp := time.Now().Add(time.Hour).Unix()
			if f.expired {
				exp = time.Now().Add(-time.Hour).Unix()
			}
			issuer := f.server.URL
			if f.issuerOverride != "" {
				issuer = f.issuerOverride
			}
			claims := map[string]any{"iss": issuer, "sub": f.subject, "aud": f.audience, "nonce": nonce, "exp": exp, "iat": time.Now().Unix(), "email": "person@example.test"}
			payload, _ := json.Marshal(claims)
			signed, err := f.signer.Sign(payload)
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			jwt, _ := signed.CompactSerialize()
			response := tokenResponse{AccessToken: "access-secret", RefreshToken: "refresh-secret", IDToken: jwt, TokenType: "Bearer", ExpiresIn: 3600, Scope: f.scope}
			if f.identityOnly {
				response.AccessToken = ""
				response.RefreshToken = ""
			}
			_ = json.NewEncoder(w).Encode(response)
		case "/revoke":
			f.revokeCount.Add(1)
			_ = r.ParseForm()
			hint := r.Form.Get("token_type_hint")
			if (hint != "refresh_token" && hint != "access_token") || r.Form.Get("client_id") != "oaiapp_test" {
				t.Error("invalid revocation parameters")
			}
			f.revokedHint.Store(hint)
			if f.revokeStatus != 0 {
				w.WriteHeader(f.revokeStatus)
				return
			}
		case "/v1/models":
			if r.Header.Get("Authorization") != "Bearer access-secret" && r.Header.Get("Authorization") != "Bearer refreshed-access" {
				t.Error("missing OAuth bearer credential")
			}
			_, _ = fmt.Fprint(w, `{"models":[{"slug":"available","display_name":"Available","visibility":"list"},{"slug":"hidden","visibility":"hide"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	f.client = New(filepath.Join(t.TempDir(), "chatgpt", "credentials.json"))
	f.client.issuer = f.server.URL
	f.client.resource = f.server.URL + "/v1"
	return f
}

func (f *identityFixture) open(t *testing.T, mutate func(url.Values)) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		f.mu.Lock()
		f.authorization = u.Query()
		f.mu.Unlock()
		if u.Query().Get("ext_agent_host_id") == "" || u.Query().Get("scope") != requestedScopes {
			t.Error("missing host ID or plan permission")
		}
		callback, err := url.Parse(u.Query().Get("redirect_uri"))
		if err != nil {
			return err
		}
		if callback.Hostname() != "127.0.0.1" || callback.Path != "/auth/callback" {
			t.Error("callback must use the exact loopback host and path")
		}
		q := url.Values{"code": {"test-code"}, "state": {u.Query().Get("state")}, "client_id": {"oaiapp_test"}}
		if mutate != nil {
			mutate(q)
		}
		callback.RawQuery = q.Encode()
		resp, err := http.Get(callback.String())
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}
}

func TestLoginAndReturningRegistration(t *testing.T) {
	f := newIdentityFixture(t)
	p, err := f.client.Login(context.Background(), "", f.open(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !p.PlanEnabled || p.ClientID != "oaiapp_test" {
		t.Fatalf("unexpected profile: %+v", p)
	}
	s, err := f.client.load()
	if err != nil {
		t.Fatal(err)
	}
	host := s.HostID
	if !strings.HasPrefix(host, "urn:uuid:") {
		t.Fatal("missing stable host ID")
	}
	info, err := os.Stat(f.client.path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("credential permissions must be owner-only")
	}
	if _, err := f.client.Login(context.Background(), p.ClientID, f.open(t, nil)); err != nil {
		t.Fatal(err)
	}
	if f.authorization.Get("client_id") != p.ClientID || f.authorization.Get("agent_name_hint") != "" || f.authorization.Get("ext_agent_host_id") != host {
		t.Fatal("returning registration must reuse its IDs")
	}
	profiles, err := f.client.Profiles()
	if err != nil || len(profiles) != 1 || !profiles[0].Active {
		t.Fatalf("profiles: %+v %v", profiles, err)
	}
	models, err := f.client.Models(context.Background())
	if err != nil || len(models) != 1 || models[0].Slug != "available" {
		t.Fatalf("models: %+v %v", models, err)
	}
	if err := f.client.Select(context.Background(), p.ClientID); err != nil {
		t.Fatal(err)
	}
	if err := f.client.Select(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown registration accepted")
	}
}

func TestLoginRejectsInvalidIdentity(t *testing.T) {
	for _, test := range []string{"audience", "expiry", "nonce", "issuer", "signature", "denied", "client_id", "missing_code", "returning_client", "returning_subject"} {
		t.Run(test, func(t *testing.T) {
			f := newIdentityFixture(t)
			var mutate func(url.Values)
			id := ""
			switch test {
			case "audience":
				f.audience = "another-client"
			case "expiry":
				f.expired = true
			case "nonce":
				f.nonceOverride = "wrong-nonce"
			case "issuer":
				f.issuerOverride = "https://another-issuer.example.test"
			case "signature":
				otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
				f.signer, err = jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: otherKey}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
				if err != nil {
					t.Fatal(err)
				}
			case "denied":
				mutate = func(q url.Values) { q.Set("error", "access_denied") }
			case "client_id":
				mutate = func(q url.Values) { q.Del("client_id") }
			case "missing_code":
				mutate = func(q url.Values) { q.Del("code") }
			case "returning_client", "returning_subject":
				if _, err := f.client.Login(context.Background(), "", f.open(t, nil)); err != nil {
					t.Fatal(err)
				}
				id = "oaiapp_test"
				if test == "returning_client" {
					mutate = func(q url.Values) { q.Set("client_id", "oaiapp_other") }
				} else {
					f.subject = "different-user"
				}
			}
			if _, err := f.client.Login(context.Background(), id, f.open(t, mutate)); err == nil {
				t.Fatal("invalid authentication accepted")
			}
			s, err := f.client.load()
			if err != nil {
				t.Fatal(err)
			}
			if id == "" && len(s.Accounts) != 0 {
				t.Fatal("invalid identity persisted")
			}
			if id != "" && s.Accounts[id].Subject != "user-1" {
				t.Fatal("active identity was replaced")
			}
		})
	}
}

func TestLoginStateAndCancellation(t *testing.T) {
	f := newIdentityFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := f.client.Login(ctx, "", f.open(t, func(q url.Values) { q.Set("state", "unrelated") }))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("mismatched state should never exchange a code: %v", err)
	}
	_, err = f.client.Login(context.Background(), "", func(string) error { return errors.New("browser stopped") })
	if err == nil {
		t.Fatal("browser failure ignored")
	}
	if _, err := f.client.Login(context.Background(), "unknown", f.open(t, nil)); err == nil {
		t.Fatal("unknown account accepted")
	}
}

func TestRefreshIsSerializedAndRotatesTokens(t *testing.T) {
	f := newIdentityFixture(t)
	if _, err := f.client.Login(context.Background(), "", f.open(t, nil)); err != nil {
		t.Fatal(err)
	}
	if err := f.client.withStore(context.Background(), func(s *store) error { s.Accounts[s.Active].ExpiresAt = time.Now(); return f.client.save(s) }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := New(f.client.path)
			c.issuer = f.client.issuer
			c.resource = f.client.resource
			token, err := c.AccessToken(context.Background())
			if err != nil || token != "refreshed-access" {
				t.Errorf("token: %s %v", token, err)
			}
		}()
	}
	wg.Wait()
	if f.refreshCount.Load() != 1 {
		t.Fatal("rotating refresh token was used concurrently")
	}
	s, err := f.client.load()
	if err != nil || s.Accounts[s.Active].RefreshToken != "rotated-refresh" {
		t.Fatalf("rotation was not persisted: %v", err)
	}
}

func TestPlanPermissionAndRefreshFailurePreserveCredentials(t *testing.T) {
	for _, test := range []string{"identity_only", "refresh_failure", "earliest_refresh"} {
		t.Run(test, func(t *testing.T) {
			f := newIdentityFixture(t)
			if test == "identity_only" {
				f.scope = "openid email offline_access"
			}
			p, err := f.client.Login(context.Background(), "", f.open(t, nil))
			if err != nil {
				t.Fatal(err)
			}
			if test == "identity_only" && p.PlanEnabled {
				t.Fatal("identity is not plan permission")
			}
			if test != "identity_only" {
				if err := f.client.withStore(context.Background(), func(s *store) error {
					a := s.Accounts[s.Active]
					a.ExpiresAt = time.Now().Add(-time.Second)
					if test == "earliest_refresh" {
						a.EarliestRefreshAt = time.Now().Add(time.Hour)
					}
					return f.client.save(s)
				}); err != nil {
					t.Fatal(err)
				}
				f.tokenStatus = 400
			}
			if _, err := f.client.AccessToken(context.Background()); err == nil {
				t.Fatal("ineligible session accepted")
			}
			s, err := f.client.load()
			if err != nil || s.Accounts[s.Active].AccessToken != "access-secret" {
				t.Fatal("credentials must survive failure")
			}
		})
	}
}

func TestLogoutClearsTokensAndRetainsRegistration(t *testing.T) {
	for _, status := range []int{0, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newIdentityFixture(t)
			if _, err := f.client.Login(context.Background(), "", f.open(t, nil)); err != nil {
				t.Fatal(err)
			}
			f.revokeStatus = status
			err := f.client.Logout(context.Background())
			if (err != nil) != (status != 0) {
				t.Fatalf("logout: %v", err)
			}
			s, e := f.client.load()
			if e != nil {
				t.Fatal(e)
			}
			a := s.Accounts["oaiapp_test"]
			if a == nil || a.AccessToken != "" || a.RefreshToken != "" || a.IDToken != "" || s.HostID == "" {
				t.Fatal("logout must clear secrets and retain registration")
			}
			if _, err := f.client.AccessToken(context.Background()); err == nil {
				t.Fatal("logged-out session accepted")
			}
			if (status == 0 && f.revokeCount.Load() != 1) || (status == 503 && f.revokeCount.Load() != 3) {
				t.Fatal("session was not revoked")
			}
		})
	}
}

func TestLogoutRevokesAccessTokenWithoutRefreshToken(t *testing.T) {
	f := newIdentityFixture(t)
	if _, err := f.client.Login(context.Background(), "", f.open(t, nil)); err != nil {
		t.Fatal(err)
	}
	if err := f.client.withStore(context.Background(), func(s *store) error { s.Accounts[s.Active].RefreshToken = ""; return f.client.save(s) }); err != nil {
		t.Fatal(err)
	}
	if err := f.client.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.revokeCount.Load() != 1 || f.revokedHint.Load() != "access_token" {
		t.Fatalf("access-only session was not revoked: %d %v", f.revokeCount.Load(), f.revokedHint.Load())
	}
}

func TestCredentialStoreErrorsAndEmptyState(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "credentials.json"))
	if profiles, err := c.Profiles(); err != nil || len(profiles) != 0 {
		t.Fatalf("empty profiles: %v", err)
	}
	if _, err := c.AccessToken(context.Background()); err == nil {
		t.Fatal("missing session accepted")
	}
	if err := c.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.path, []byte(`invalid`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Profiles(); err == nil {
		t.Fatal("invalid store accepted")
	}
	if err := c.applyToken(&account{}, tokenResponse{}, false); err == nil {
		t.Fatal("incomplete token accepted")
	}
}

func TestIdentityOnlyGrantAndPinnedAccount(t *testing.T) {
	f := newIdentityFixture(t)
	f.scope = "openid email"
	f.identityOnly = true
	profile, err := f.client.Login(context.Background(), "", f.open(t, nil))
	if err != nil || profile.PlanEnabled {
		t.Fatalf("identity-only sign-in: %+v %v", profile, err)
	}
	if _, err := f.client.AccessToken(context.Background()); err == nil || !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("identity must not permit inference: %v", err)
	}
	f.identityOnly = false
	f.scope = requestedScopes
	if _, err := f.client.Login(context.Background(), "oaiapp_test", f.open(t, nil)); err != nil {
		t.Fatal(err)
	}
	if f.authorization.Get("prompt") != "consent" {
		t.Fatal("enabling plan usage must request consent")
	}
	source := f.client.TokenSource()
	if token, err := source(context.Background()); err != nil || token != "access-secret" {
		t.Fatalf("token: %v", err)
	}
	if err := f.client.withStore(context.Background(), func(s *store) error {
		other := *s.Accounts[s.Active]
		other.ClientID = "oaiapp_other"
		other.AccessToken = "other-token"
		s.Accounts[other.ClientID] = &other
		s.Active = other.ClientID
		return f.client.save(s)
	}); err != nil {
		t.Fatal(err)
	}
	if token, err := source(context.Background()); err != nil || token != "access-secret" {
		t.Fatal("running review switched billing accounts")
	}
	if token, err := f.client.AccessToken(context.Background()); err != nil || token != "other-token" {
		t.Fatal("new request did not use selected account")
	}
}

func TestRefreshWithoutRotationKeepsRefreshToken(t *testing.T) {
	f := newIdentityFixture(t)
	if _, err := f.client.Login(context.Background(), "", f.open(t, nil)); err != nil {
		t.Fatal(err)
	}
	if err := f.client.withStore(context.Background(), func(s *store) error { s.Accounts[s.Active].ExpiresAt = time.Now(); return f.client.save(s) }); err != nil {
		t.Fatal(err)
	}
	f.noRotation = true
	if token, err := f.client.AccessToken(context.Background()); err != nil || token != "refreshed-access" {
		t.Fatalf("refresh without a new refresh token must succeed: %s %v", token, err)
	}
	s, err := f.client.load()
	if err != nil || s.Accounts[s.Active].RefreshToken != "refresh-secret" {
		t.Fatalf("existing refresh token must be retained: %v", err)
	}
}

func TestLoginWithOmittedScopeKeepsRequestedScopes(t *testing.T) {
	f := newIdentityFixture(t)
	f.scope = ""
	profile, err := f.client.Login(context.Background(), "", f.open(t, nil))
	if err != nil || !profile.PlanEnabled {
		t.Fatalf("omitted scope must mean the requested scopes were granted: %+v %v", profile, err)
	}
	if _, err := f.client.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReloginWithoutPlanKeepsSameRegistration(t *testing.T) {
	f := newIdentityFixture(t)
	if _, err := f.client.Login(context.Background(), "", f.open(t, nil)); err != nil {
		t.Fatal(err)
	}
	before, err := f.client.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.scope = "openid email offline_access"
	if _, err := f.client.Login(context.Background(), "oaiapp_test", f.open(t, nil)); err == nil {
		t.Fatal("a grant without plan usage must not replace a plan-enabled registration")
	}
	if token, err := f.client.AccessToken(context.Background()); err != nil || token != before {
		t.Fatalf("plan-enabled registration was overwritten: %s %v", token, err)
	}
}

func TestIdentityOnlyGrantKeepsPlanEnabledActiveAccount(t *testing.T) {
	f := newIdentityFixture(t)
	if err := f.client.withStore(context.Background(), func(s *store) error {
		s.Accounts["oaiapp_working"] = &account{ClientID: "oaiapp_working", Issuer: f.client.issuer, Subject: "user-2", AccessToken: "working-token", Scopes: []string{PlanScope}, ExpiresAt: time.Now().Add(time.Hour)}
		s.Active = "oaiapp_working"
		return f.client.save(s)
	}); err != nil {
		t.Fatal(err)
	}
	f.scope = "openid email"
	f.identityOnly = true
	profile, err := f.client.Login(context.Background(), "", f.open(t, nil))
	if err != nil || profile.PlanEnabled || profile.Active {
		t.Fatalf("identity-only registration must not become active: %+v %v", profile, err)
	}
	if token, err := f.client.AccessToken(context.Background()); err != nil || token != "working-token" {
		t.Fatalf("working account was displaced: %s %v", token, err)
	}
}
