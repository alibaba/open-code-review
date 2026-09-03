// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/codexauth"
)

type commandAuthStore struct {
	auth     *codexauth.CodexAuth
	loadErr  error
	clearErr error
	cleared  bool
}

func (s *commandAuthStore) Load() (*codexauth.CodexAuth, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.auth, nil
}

func (s *commandAuthStore) Save(auth *codexauth.CodexAuth) error {
	s.auth = auth
	return nil
}

func (s *commandAuthStore) Clear() error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.cleared = true
	s.auth = nil
	return nil
}

func TestRunAuthStatusMasksCredentialMetadata(t *testing.T) {
	const access = "access-token-must-not-appear"
	const refresh = "refresh-token-must-not-appear"
	expires := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.Local)
	store := &commandAuthStore{auth: &codexauth.CodexAuth{
		AccessToken: access, RefreshToken: refresh, AccountID: "acct_1234567890", PlanType: "plus", ExpiresAt: expires,
	}}
	var output bytes.Buffer
	if err := runAuthStatus(&output, store, expires.Add(-time.Hour)); err != nil {
		t.Fatalf("runAuthStatus: %v", err)
	}
	got := output.String()
	for _, want := range []string{"acct***7890", "plus", expires.Format(time.RFC3339)} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, access) || strings.Contains(got, refresh) || strings.Contains(got, "acct_1234567890") {
		t.Fatalf("status output exposed credentials: %q", got)
	}
}

func TestRunAuthStatusReportsSignedOutWithoutError(t *testing.T) {
	store := &commandAuthStore{loadErr: codexauth.ErrNotFound}
	var output bytes.Buffer
	if err := runAuthStatus(&output, store, time.Now()); err != nil {
		t.Fatalf("runAuthStatus: %v", err)
	}
	if got := output.String(); got != "Not signed in, run ocr auth login.\n" {
		t.Errorf("status output = %q", got)
	}
}

func TestRunAuthStatusMarksExpiredAndReportsLoadFailure(t *testing.T) {
	now := time.Now()
	store := &commandAuthStore{auth: &codexauth.CodexAuth{AccessToken: "secret", ExpiresAt: now.Add(-time.Minute)}}
	var output bytes.Buffer
	if err := runAuthStatus(&output, store, now); err != nil {
		t.Fatalf("runAuthStatus: %v", err)
	}
	if !strings.Contains(output.String(), "(expired)") || !strings.Contains(output.String(), "(unknown)") {
		t.Errorf("status output = %q", output.String())
	}
	store.loadErr = errors.New("broken store")
	if err := runAuthStatus(&output, store, now); err == nil || !strings.Contains(err.Error(), "broken store") {
		t.Errorf("load error = %v", err)
	}
}

func TestRunAuthLogoutRevokesAndClears(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store := &commandAuthStore{auth: &codexauth.CodexAuth{AccessToken: "access", RefreshToken: "refresh"}}
	client := &codexauth.OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	var output bytes.Buffer
	if err := runAuthLogout(context.Background(), &output, store, client); err != nil {
		t.Fatalf("runAuthLogout: %v", err)
	}
	if !store.cleared || !strings.Contains(output.String(), "refresh token was revoked") ||
		!strings.Contains(output.String(), "up to ten days") {
		t.Errorf("cleared = %t, output = %q", store.cleared, output.String())
	}
}

func TestRunAuthLogoutClearsAfterRevocationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	store := &commandAuthStore{auth: &codexauth.CodexAuth{AccessToken: "access", RefreshToken: "refresh"}}
	client := &codexauth.OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	var output bytes.Buffer
	if err := runAuthLogout(context.Background(), &output, store, client); err != nil {
		t.Fatalf("runAuthLogout: %v", err)
	}
	if !store.cleared || !strings.Contains(output.String(), "server-side revocation failed") ||
		!strings.Contains(output.String(), "up to ten days") {
		t.Errorf("cleared = %t, output = %q", store.cleared, output.String())
	}
}

func TestRunAuthLogoutDistinguishesLoadFailure(t *testing.T) {
	store := &commandAuthStore{loadErr: errors.New("broken credential file")}
	var output bytes.Buffer
	if err := runAuthLogout(context.Background(), &output, store, codexauth.NewOAuthClient()); err != nil {
		t.Fatalf("runAuthLogout: %v", err)
	}
	if !store.cleared || !strings.Contains(output.String(), "loading them") ||
		strings.Contains(output.String(), "but server-side revocation failed") ||
		!strings.Contains(output.String(), "up to ten days") {
		t.Errorf("cleared = %t, output = %q", store.cleared, output.String())
	}
}

func TestRunAuthLogoutReportsClearFailure(t *testing.T) {
	store := &commandAuthStore{loadErr: codexauth.ErrNotFound, clearErr: errors.New("clear failed")}
	err := runAuthLogout(context.Background(), &bytes.Buffer{}, store, codexauth.NewOAuthClient())
	if err == nil || !strings.Contains(err.Error(), "clear failed") {
		t.Errorf("runAuthLogout error = %v", err)
	}
}

func TestAuthLoginFlagsAreMutuallyExclusive(t *testing.T) {
	rootCmd.SetArgs([]string{"auth", "login", "--device", "--no-browser"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		for _, name := range []string{"device", "no-browser"} {
			flag := authLoginCmd.Flags().Lookup(name)
			_ = flag.Value.Set("false")
			flag.Changed = false
		}
	})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "group [device no-browser]") {
		t.Errorf("Execute error = %v", err)
	}
}

func TestMaskedAccount(t *testing.T) {
	for input, want := range map[string]string{
		"":            "(unknown)",
		"short":       "***",
		"acct_123456": "acct***3456",
	} {
		if got := maskedAccount(input); got != want {
			t.Errorf("maskedAccount(%q) = %q, want %q", input, got, want)
		}
	}
}
