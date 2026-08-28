// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package codexauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryStore struct {
	mu      sync.Mutex
	auth    *CodexAuth
	loadErr error
	saveErr error
	saves   int
}

func (s *memoryStore) Load() (*CodexAuth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.auth == nil {
		return nil, errors.New("missing")
	}
	copy := *s.auth
	return &copy, nil
}

func (s *memoryStore) Save(auth *CodexAuth) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	copy := *auth
	s.auth = &copy
	s.saves++
	return nil
}

func (s *memoryStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auth = nil
	return nil
}

func makeJWT(t *testing.T, accountID, plan string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"https://api.openai.com/auth": map[string]string{
			"chatgpt_account_id": accountID,
			"chatgpt_plan_type":  plan,
		},
	})
	if err != nil {
		t.Fatalf("Marshal JWT: %v", err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func writeTokenResponse(t *testing.T, w http.ResponseWriter, access, refresh, id string, expires int64) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tokenResponse{
		AccessToken: access, RefreshToken: refresh, IDToken: id, ExpiresIn: expires,
	}); err != nil {
		t.Errorf("Encode token response: %v", err)
	}
}

func TestNewOAuthClientHasBoundedHTTPTimeout(t *testing.T) {
	client := NewOAuthClient()
	if client.HTTPClient.Timeout != 30*time.Second {
		t.Errorf("HTTP timeout = %v, want 30s", client.HTTPClient.Timeout)
	}
}

func TestAuthorizeURLContainsCodexOAuthContract(t *testing.T) {
	client := &OAuthClient{Issuer: "https://issuer.example/"}
	got := client.authorizeURL(pkceCodes{challenge: "challenge"}, "state-value")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Scheme+"://"+parsed.Host+parsed.Path != "https://issuer.example/oauth/authorize" {
		t.Errorf("authorize endpoint = %q", parsed.String())
	}
	want := map[string]string{
		"response_type":              "code",
		"client_id":                  ClientID,
		"redirect_uri":               RedirectURI,
		"scope":                      Scope,
		"code_challenge":             "challenge",
		"code_challenge_method":      "S256",
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
		"state":                      "state-value",
		"originator":                 "codex_cli_rs",
	}
	for key, value := range want {
		if parsed.Query().Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, parsed.Query().Get(key), value)
		}
	}
}

func TestCallbackHandlerValidation(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		query      string
		wantStatus int
		wantCode   string
		wantErr    string
	}{
		{name: "success", host: "localhost:1455", query: "state=expected&code=code-value", wantStatus: http.StatusOK, wantCode: "code-value"},
		{name: "IPv4 loopback", host: "127.0.0.1:1455", query: "state=expected&code=code-value", wantStatus: http.StatusOK, wantCode: "code-value"},
		{name: "forbidden host", host: "attacker.example", query: "state=expected&code=code-value", wantStatus: http.StatusForbidden},
		{name: "state mismatch", host: "localhost:1455", query: "state=wrong&code=code-value", wantStatus: http.StatusBadRequest},
		{name: "denied", host: "localhost:1455", query: "state=expected&error=access_denied", wantStatus: http.StatusBadRequest, wantErr: "denied"},
		{name: "missing code", host: "localhost:1455", query: "state=expected", wantStatus: http.StatusBadRequest, wantErr: "authorization code"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := make(chan callbackResult, 1)
			req := httptest.NewRequest(http.MethodGet, "/auth/callback?"+tc.query, nil)
			req.Host = tc.host
			recorder := httptest.NewRecorder()
			callbackHandler("expected", result).ServeHTTP(recorder, req)
			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			if tc.wantCode == "" && tc.wantErr == "" {
				select {
				case got := <-result:
					t.Fatalf("unexpected callback result: %#v", got)
				default:
				}
				return
			}
			got := <-result
			if got.code != tc.wantCode {
				t.Errorf("code = %q, want %q", got.code, tc.wantCode)
			}
			if tc.wantErr != "" && (got.err == nil || !strings.Contains(got.err.Error(), tc.wantErr)) {
				t.Errorf("error = %v, want substring %q", got.err, tc.wantErr)
			}
		})
	}
}

func TestCallbackHandlerKeepsListeningAfterInvalidState(t *testing.T) {
	result := make(chan callbackResult, 1)
	handler := callbackHandler("expected", result)

	badRequest := httptest.NewRequest(http.MethodGet, "/auth/callback?state=wrong&code=attacker", nil)
	badRequest.Host = "localhost:1455"
	badResponse := httptest.NewRecorder()
	handler.ServeHTTP(badResponse, badRequest)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("bad callback status = %d, want %d", badResponse.Code, http.StatusBadRequest)
	}
	select {
	case got := <-result:
		t.Fatalf("invalid state completed login: %#v", got)
	default:
	}

	validRequest := httptest.NewRequest(http.MethodGet, "/auth/callback?state=expected&code=legitimate", nil)
	validRequest.Host = "localhost:1455"
	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, validRequest)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("valid callback status = %d, want %d", validResponse.Code, http.StatusOK)
	}
	if got := <-result; got.code != "legitimate" || got.err != nil {
		t.Fatalf("valid callback result = %#v", got)
	}
}

func TestLoginLoopbackUsesBrowserAndSavesTokens(t *testing.T) {
	jwt := makeJWT(t, "acct_123", "plus")
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "callback-code" {
			t.Errorf("unexpected token form: %v", r.Form)
		}
		if r.Form.Get("code_verifier") == "" {
			t.Error("code_verifier is empty")
		}
		writeTokenResponse(t, w, "new-access", "new-refresh", jwt, 3600)
	}))
	defer tokenServer.Close()

	originalOpenBrowser := openBrowser
	t.Cleanup(func() { openBrowser = originalOpenBrowser })
	openBrowser = func(target string) error {
		parsed, err := url.Parse(target)
		if err != nil {
			return err
		}
		callback := RedirectURI + "?state=" + url.QueryEscape(parsed.Query().Get("state")) + "&code=callback-code"
		response, err := http.Get(callback)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, response.Body)
		return response.Body.Close()
	}

	store := &memoryStore{}
	client := &OAuthClient{Issuer: tokenServer.URL, HTTPClient: tokenServer.Client()}
	auth, err := client.LoginLoopback(context.Background(), store, false, io.Discard)
	if err != nil {
		t.Fatalf("LoginLoopback: %v", err)
	}
	if auth.AccessToken != "new-access" || auth.RefreshToken != "new-refresh" {
		t.Errorf("auth = %#v", auth)
	}
	if auth.AccountID != "acct_123" || auth.PlanType != "plus" {
		t.Errorf("metadata = account %q, plan %q", auth.AccountID, auth.PlanType)
	}
	if store.saves != 1 {
		t.Errorf("Save calls = %d, want 1", store.saves)
	}
}

func TestExchangeCodeErrorsNeverContainTokens(t *testing.T) {
	const access = "access-token-must-not-leak"
	const refresh = "refresh-token-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"`+access+` `+refresh+`"}`)
	}))
	defer server.Close()

	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	_, err := client.exchangeCode(context.Background(), "code", RedirectURI, "verifier", time.Now())
	if err == nil {
		t.Fatal("exchangeCode succeeded")
	}
	if strings.Contains(err.Error(), access) || strings.Contains(err.Error(), refresh) {
		t.Fatalf("error leaked a token: %v", err)
	}
}

func TestAuthFromTokenResponseRejectsExcessiveLifetime(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	_, err := authFromTokenResponse(tokenResponse{
		AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 1<<63 - 1,
	}, now)
	if err == nil || !strings.Contains(err.Error(), "lifetime exceeds") {
		t.Fatalf("authFromTokenResponse error = %v, want excessive lifetime error", err)
	}
}

func TestRefreshIfNeededRotatesAndPersistsTokens(t *testing.T) {
	setTestHome(t)
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	store := &memoryStore{auth: &CodexAuth{
		AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: now.Add(time.Minute),
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			GrantType    string `json:"grant_type"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("Decode refresh request: %v", err)
		}
		if request.GrantType != "refresh_token" || request.RefreshToken != "old-refresh" {
			t.Errorf("refresh request = %#v", request)
		}
		writeTokenResponse(t, w, "rotated-access", "rotated-refresh", "", 7200)
	}))
	defer server.Close()

	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	got, err := client.RefreshIfNeeded(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatalf("RefreshIfNeeded: %v", err)
	}
	if got.AccessToken != "rotated-access" || got.RefreshToken != "rotated-refresh" {
		t.Errorf("rotated auth = %#v", got)
	}
	stored, _ := store.Load()
	if stored.RefreshToken != "rotated-refresh" || store.saves != 1 {
		t.Errorf("stored auth = %#v, saves = %d", stored, store.saves)
	}
}

func TestRefreshIfNeededKeepsExistingRefreshTokenWhenResponseOmitsIt(t *testing.T) {
	setTestHome(t)
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	store := &memoryStore{auth: &CodexAuth{
		AccessToken: "old-access", RefreshToken: "existing-refresh", ExpiresAt: now.Add(time.Minute),
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTokenResponse(t, w, "new-access", "", "", 3600)
	}))
	defer server.Close()

	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	got, err := client.RefreshIfNeeded(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatalf("RefreshIfNeeded: %v", err)
	}
	if got.RefreshToken != "existing-refresh" {
		t.Errorf("RefreshToken = %q, want existing-refresh", got.RefreshToken)
	}
	stored, _ := store.Load()
	if stored.RefreshToken != "existing-refresh" || store.saves != 1 {
		t.Errorf("stored auth = %#v, saves = %d", stored, store.saves)
	}
}

func TestRefreshIfNeededRechecksAfterCrossProcessLock(t *testing.T) {
	setTestHome(t)
	now := time.Now()
	store := &memoryStore{auth: &CodexAuth{
		AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: now.Add(time.Minute),
	}}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(75 * time.Millisecond)
		writeTokenResponse(t, w, "new-access", "new-refresh", "", 7200)
	}))
	defer server.Close()
	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}

	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := client.RefreshIfNeeded(context.Background(), store, func() time.Time { return now })
			errs <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Errorf("RefreshIfNeeded: %v", err)
		}
	}
	if requests.Load() != 1 {
		t.Errorf("refresh requests = %d, want 1", requests.Load())
	}
}

func TestRefreshIfNeededSkipsFreshToken(t *testing.T) {
	now := time.Now()
	want := &CodexAuth{AccessToken: "fresh", ExpiresAt: now.Add(time.Hour)}
	store := &memoryStore{auth: want}
	got, err := NewOAuthClient().RefreshIfNeeded(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatalf("RefreshIfNeeded: %v", err)
	}
	if got.AccessToken != want.AccessToken || store.saves != 0 {
		t.Errorf("got %#v, saves %d", got, store.saves)
	}
}

func TestRefreshIfNeededRefreshesZeroExpiry(t *testing.T) {
	setTestHome(t)
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	store := &memoryStore{auth: &CodexAuth{
		AccessToken: "old-access", RefreshToken: "old-refresh",
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTokenResponse(t, w, "new-access", "new-refresh", "", 3600)
	}))
	defer server.Close()

	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	got, err := client.RefreshIfNeeded(context.Background(), store, func() time.Time { return now })
	if err != nil {
		t.Fatalf("RefreshIfNeeded: %v", err)
	}
	if got.AccessToken != "new-access" || store.saves != 1 {
		t.Errorf("got %#v, saves %d", got, store.saves)
	}
}

func TestAcquireRefreshLockBreaksStaleLock(t *testing.T) {
	setTestHome(t)
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := path + ".lock"
	const absentPID = 1 << 30
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(absentPID)), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	staleTime := time.Now().Add(-2 * refreshLockStaleAfter)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	release, err := acquireRefreshLock(context.Background())
	if err != nil {
		t.Fatalf("acquireRefreshLock: %v", err)
	}
	defer release()
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("active lock does not exist: %v", err)
	}
}

func TestAcquireRefreshLockRejectsStaleLockWithoutOwner(t *testing.T) {
	setTestHome(t)
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := path + ".lock"
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	staleTime := time.Now().Add(-2 * refreshLockStaleAfter)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	_, err = acquireRefreshLock(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no valid owner PID") {
		t.Fatalf("acquireRefreshLock error = %v, want invalid owner guidance", err)
	}
}

func TestAcquireRefreshLockDoesNotBreakStaleLockOwnedByLiveProcess(t *testing.T) {
	setTestHome(t)
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockPath := path + ".lock"
	owner := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(lockPath, []byte(owner), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	staleTime := time.Now().Add(-2 * refreshLockStaleAfter)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = acquireRefreshLock(ctx)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireRefreshLock error = %v, want deadline exceeded", err)
	}
	data, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(data) != owner {
		t.Errorf("lock owner = %q, want %q", data, owner)
	}
}

func TestAcquireRefreshLockHonorsContextDeadline(t *testing.T) {
	setTestHome(t)
	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = acquireRefreshLock(ctx)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireRefreshLock error = %v, want deadline exceeded", err)
	}
}

func TestRevoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/revoke" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Form.Get("token") != "refresh-value" || r.Form.Get("token_type_hint") != "refresh_token" {
			t.Errorf("revoke form = %v", r.Form)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	if err := client.Revoke(context.Background(), &CodexAuth{RefreshToken: "refresh-value"}); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := client.Revoke(context.Background(), nil); err == nil {
		t.Fatal("Revoke(nil) succeeded")
	}
}

func TestRefreshIfNeededPreservesIdentityWhenIDTokenOmitted(t *testing.T) {
	setTestHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A provider that does not re-issue the id_token on refresh.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":864000}`))
	}))
	defer server.Close()

	store := FileStore{}
	existing := &CodexAuth{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		IDToken:      "header.payload.signature",
		AccountID:    "acct-123",
		PlanType:     "plus",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
	if err := store.Save(existing); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	got, err := client.RefreshIfNeeded(context.Background(), store, time.Now)
	if err != nil {
		t.Fatalf("RefreshIfNeeded returned error: %v", err)
	}
	if got.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "new-access")
	}
	if got.IDToken != existing.IDToken {
		t.Errorf("IDToken = %q, want the stored token %q", got.IDToken, existing.IDToken)
	}
}
