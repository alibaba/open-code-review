// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"crypto"
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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testStore(t *testing.T) Store { t.Helper(); return Store{Dir: filepath.Join(t.TempDir(), "auth")} }
func testAuth() Auth {
	return Auth{ClientID: "oaiapp_one", Issuer: Issuer, Subject: "subject", Nonce: "nonce", Email: "test@example.invalid", AccessToken: "access", RefreshToken: "refresh", IDToken: "id", Scopes: strings.Fields(Scope), ExpiresAt: time.Now().Add(time.Hour), TokenType: "Bearer"}
}
func saveTest(t *testing.T, s Store, a Auth) {
	t.Helper()
	if err := s.withLock(context.Background(), func() error {
		d, err := s.load()
		if err != nil {
			return err
		}
		d.Accounts = append(d.Accounts, a)
		d.Active = a.ClientID
		return s.save(d)
	}); err != nil {
		t.Fatal(err)
	}
}
func TestStoreLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	h, err := s.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "urn:uuid:") {
		t.Fatal(h)
	}
	again, err := s.Host(ctx)
	if err != nil || h != again {
		t.Fatal(again, err)
	}
	a := testAuth()
	saveTest(t, s, a)
	b := a
	b.ClientID = "oaiapp_two"
	saveTest(t, s, b)
	d, err := s.Snapshot()
	if err != nil || len(d.Accounts) != 2 || d.Active != b.ClientID {
		t.Fatal(d, err)
	}
	for _, f := range []string{"accounts.json", "host.json", "session.lock"} {
		info, err := os.Stat(filepath.Join(s.Dir, f))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal(f, info, err)
		}
	}
	info, _ := os.Stat(s.Dir)
	if info.Mode().Perm() != 0700 {
		t.Fatal(info.Mode())
	}
	if err := s.Clear(ctx, b.ClientID); err != nil {
		t.Fatal(err)
	}
	a2, err := s.Selected("")
	if err != nil || a2.AccessToken != "" || a2.IDToken != "" || a2.Subject != a.Subject {
		t.Fatal(a2, err)
	}
	h2, _ := s.Host(ctx)
	if h2 != h {
		t.Fatal(h2)
	}
}
func TestStoreCorruptionAndLockCancellation(t *testing.T) {
	s := testStore(t)
	if _, err := s.Selected(""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.withLock(context.Background(), func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		return s.withLock(ctx, func() error { t.Fatal("lock acquired twice"); return nil })
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "accounts.json"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Snapshot(); err == nil {
		t.Fatal("corrupt DB accepted")
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "host.json"), []byte(`{"id":"bad"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Host(context.Background()); err == nil {
		t.Fatal("bad host accepted")
	}
}

type oauthFixture struct {
	client *Client
	key    *rsa.PrivateKey
	server *httptest.Server
	nonce  string
	claims map[string]any
	calls  atomic.Int32
	token  func(*http.Request) any
}

func fixture(t *testing.T) *oauthFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &oauthFixture{key: key, nonce: "nonce"}
	f.claims = map[string]any{"iss": Issuer, "sub": "subject", "aud": "oaiapp_one", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": f.nonce, "email": "test@example.invalid"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": Issuer, "jwks_uri": f.server.URL + "/.well-known/jwks.json", "revocation_endpoint": f.server.URL + "/revoke"})
		case "/.well-known/jwks.json":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kid": "key", "kty": "RSA", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		case "/api/accounts/oauth/token":
			f.calls.Add(1)
			r.ParseForm()
			if f.token != nil {
				json.NewEncoder(w).Encode(f.token(r))
				return
			}
			json.NewEncoder(w).Encode(f.tokens())
		case "/revoke":
			r.ParseForm()
			if r.Form.Get("client_id") != "oaiapp_one" || r.Form.Get("token") != "refresh" {
				t.Error("bad revocation form")
			}
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	f.client = NewClient()
	f.client.base = f.server.URL
	f.client.http = f.server.Client()
	return f
}
func (f *oauthFixture) jwt() string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"key"}`))
	b, _ := json.Marshal(f.claims)
	p := h + "." + base64.RawURLEncoding.EncodeToString(b)
	d := sha256.Sum256([]byte(p))
	s, _ := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, d[:])
	return p + "." + base64.RawURLEncoding.EncodeToString(s)
}
func (f *oauthFixture) tokens() map[string]any {
	return map[string]any{"access_token": "access-new", "refresh_token": "refresh-new", "id_token": f.jwt(), "token_type": "Bearer", "expires_in": 3600, "scope": Scope}
}
func TestVerifyIDToken(t *testing.T) {
	f := fixture(t)
	if _, err := f.client.verify(context.Background(), f.jwt(), "oaiapp_one", "nonce"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		field string
		value any
	}{{"iss", "other"}, {"aud", "wrong"}, {"exp", int64(1)}, {"nonce", "wrong"}, {"sub", ""}, {"iat", nil}, {"nbf", time.Now().Add(time.Hour).Unix()}, {"aud", []string{"oaiapp_one", "other"}}, {"azp", "other"}} {
		t.Run(tc.field+fmt.Sprint(tc.value), func(t *testing.T) {
			old, ok := f.claims[tc.field]
			f.claims[tc.field] = tc.value
			defer func() {
				if ok {
					f.claims[tc.field] = old
				} else {
					delete(f.claims, tc.field)
				}
			}()
			if _, err := f.client.verify(context.Background(), f.jwt(), "oaiapp_one", "nonce"); err == nil {
				t.Fatal("invalid claim accepted")
			}
		})
	}
	good := f.jwt()
	parts := strings.Split(good, ".")
	parts[2] = base64.RawURLEncoding.EncodeToString([]byte("bad"))
	if _, err := f.client.verify(context.Background(), strings.Join(parts, "."), "oaiapp_one", "nonce"); err == nil {
		t.Fatal("bad signature accepted")
	}
	for _, bad := range []string{"bad", "e30.e30.e30", strings.Replace(good, parts[0], base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"key"}`)), 1)} {
		if _, err := f.client.verify(context.Background(), bad, "oaiapp_one", "nonce"); err == nil {
			t.Fatal("bad JWT accepted")
		}
	}
	f.claims["aud"] = []string{"oaiapp_one", "other"}
	f.claims["azp"] = "oaiapp_one"
	if _, err := f.client.verify(context.Background(), f.jwt(), "oaiapp_one", "nonce"); err != nil {
		t.Fatal(err)
	}
}
func TestAuthorizationAndCallbacks(t *testing.T) {
	c := NewClient()
	a := attempt{state: "state", nonce: "nonce", verifier: "verifier", redirect: "http://127.0.0.1:123/auth/callback", host: "urn:uuid:fixture"}
	u, _ := url.Parse(c.authorize(a, nil))
	q := u.Query()
	if q.Get("client_id") != DynamicClient || q.Get("agent_name_hint") != "OpenCodeReview" || q.Get("resource") != Resource || q.Get("scope") != Scope || q.Get("nonce") != "nonce" || q.Get("code_challenge") == "" {
		t.Fatal(q)
	}
	old := testAuth()
	u, _ = url.Parse(c.authorize(a, &old))
	q = u.Query()
	if q.Get("client_id") != old.ClientID || q.Get("agent_name_hint") != "" || q.Get("id_token_hint") != "id" {
		t.Fatal(q)
	}
	old.IDToken = ""
	u, _ = url.Parse(c.authorize(a, &old))
	if u.Query().Get("id_token_hint") != "" {
		t.Fatal("logout hint")
	}
	for _, tc := range []struct {
		query string
		old   *Auth
		ok    bool
	}{{"state=state&code=code&client_id=oaiapp_one", nil, true}, {"state=state&code=code", nil, false}, {"state=wrong&code=code&client_id=oaiapp_one", nil, false}, {"state=state&error=access_denied", nil, false}, {"state=state&code=code&client_id=dynamic_agent_client", nil, false}, {"state=state&code=code", &old, true}, {"state=state&code=code&client_id=oaiapp_other", &old, false}, {"state=state&state=state&code=code", &old, false}} {
		q, _ := url.ParseQuery(tc.query)
		_, _, err := a.callback(q, tc.old)
		if (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.query, err)
		}
	}
}
func TestExchangeIdentityAndScopes(t *testing.T) {
	f := fixture(t)
	a := attempt{nonce: "nonce", verifier: "verifier", redirect: "http://127.0.0.1:44/auth/callback"}
	f.token = func(r *http.Request) any {
		if r.Form.Get("resource") != Resource || r.Form.Get("client_id") != "oaiapp_one" || r.Form.Get("redirect_uri") != a.redirect || r.Form.Get("code_verifier") != a.verifier {
			t.Error("bad code form")
		}
		return f.tokens()
	}
	auth, err := f.client.exchange(context.Background(), a, "code", "oaiapp_one", nil)
	if err != nil || !auth.PlanEnabled() {
		t.Fatal(auth, err)
	}
	old := testAuth()
	f.claims["sub"] = "other"
	if _, err := f.client.exchange(context.Background(), a, "code", "oaiapp_one", &old); err == nil {
		t.Fatal("identity changed")
	}
	f.claims["sub"] = "subject"
	f.token = func(r *http.Request) any { b := f.tokens(); b["scope"] = "openid email"; return b }
	auth, err = f.client.exchange(context.Background(), a, "code", "oaiapp_one", nil)
	if err != nil || auth.PlanEnabled() {
		t.Fatal(auth, err)
	}
}
func TestRefreshRotationAndConcurrency(t *testing.T) {
	f := fixture(t)
	s := testStore(t)
	a := testAuth()
	a.ExpiresAt = time.Now().Add(-time.Minute)
	saveTest(t, s, a)
	f.token = func(r *http.Request) any {
		if r.Form.Get("scope") != "" || r.Form.Get("resource") != Resource || r.Form.Get("client_id") != a.ClientID || r.Form.Get("refresh_token") != "refresh" {
			t.Error("bad refresh form")
		}
		b := f.tokens()
		delete(b, "id_token")
		delete(b, "scope")
		return b
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.client.Credentials(context.Background(), s, "")
			if err != nil || r.RefreshToken != "refresh-new" || r.IDToken != "id" || !r.PlanEnabled() {
				t.Error(r, err)
			}
		}()
	}
	wg.Wait()
	if f.calls.Load() != 1 {
		t.Fatal(f.calls.Load())
	}
}
func TestRefreshRejectsIncompleteRotation(t *testing.T) {
	for _, field := range []string{"access_token", "refresh_token", "expires_in", "token_type"} {
		t.Run(field, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			a.ExpiresAt = time.Now().Add(-time.Minute)
			saveTest(t, s, a)
			f.token = func(r *http.Request) any { b := f.tokens(); delete(b, field); return b }
			if _, err := f.client.Credentials(context.Background(), s, ""); err == nil {
				t.Fatal("incomplete rotation accepted")
			}
			old, _ := s.Selected("")
			if old.RefreshToken != "refresh" {
				t.Fatal("prior tokens lost")
			}
		})
	}
}
func TestLogout(t *testing.T) {
	f := fixture(t)
	s := testStore(t)
	saveTest(t, s, testAuth())
	if err := f.client.Logout(context.Background(), s, ""); err != nil {
		t.Fatal(err)
	}
	a, err := s.Selected("")
	if err != nil || a.AccessToken != "" || a.ClientID != "oaiapp_one" {
		t.Fatal(a, err)
	}
}

func TestSigningKeyCacheAndCancellation(t *testing.T) {
	f := fixture(t)
	transport := f.client.http.Transport
	var discoveries, sets atomic.Int32
	f.client.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			discoveries.Add(1)
		case "/.well-known/jwks.json":
			sets.Add(1)
		}
		return transport.RoundTrip(r)
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, err := f.client.signingKey(context.Background(), "key")
			if err != nil || key == nil || key.N.Cmp(f.key.N) != 0 {
				t.Error("cached signing key did not match", err)
			}
		}()
	}
	wg.Wait()
	if discoveries.Load() != 1 || sets.Load() != 1 {
		t.Fatal("concurrent lookups duplicated key discovery", discoveries.Load(), sets.Load())
	}
	f.client.keys.until = time.Now().Add(-time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.client.signingKey(ctx, "key"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled discovery did not fail", err)
	}
	if _, err := f.client.signingKey(context.Background(), "key"); err != nil {
		t.Fatal("discovery did not recover after cancellation", err)
	}
	if sets.Load() != 2 {
		t.Fatal("expired cache did not fetch replacement keys", sets.Load())
	}
}
