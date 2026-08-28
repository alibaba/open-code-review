// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package codexauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestDeviceCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/accounts/deviceauth/usercode" {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Decode request: %v", err)
		}
		if body["client_id"] != ClientID {
			t.Errorf("client_id = %q", body["client_id"])
		}
		_, _ = io.WriteString(w, `{"device_auth_id":"device-1","user_code":"CODE-123","interval":"2"}`)
	}))
	defer server.Close()
	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	got, err := client.requestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("requestDeviceCode: %v", err)
	}
	if got.DeviceAuthID != "device-1" || got.UserCode != "CODE-123" || got.Interval != 2*time.Second {
		t.Errorf("device code = %#v", got)
	}
	if got.VerificationURL != server.URL+"/codex/device" {
		t.Errorf("verification URL = %q", got.VerificationURL)
	}
}

func TestPollDeviceCodePendingThenSuccess(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Decode request: %v", err)
		}
		if body["device_auth_id"] != "device-1" || body["user_code"] != "CODE-123" {
			t.Errorf("poll body = %v", body)
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
			return
		}
		_, _ = io.WriteString(w, `{"authorization_code":"auth-code","code_verifier":"verifier","code_challenge":"challenge"}`)
	}))
	defer server.Close()
	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	got, err := client.pollDeviceCode(context.Background(), "device-1", "CODE-123", 0, time.Now)
	if err != nil {
		t.Fatalf("pollDeviceCode: %v", err)
	}
	if got.AuthorizationCode != "auth-code" || got.CodeVerifier != "verifier" {
		t.Errorf("poll result = %#v", got)
	}
}

func TestPollDeviceCodeSlowDownAndExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"slow_down"}`)
	}))
	defer server.Close()
	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	start := time.Now()
	calls := 0
	now := func() time.Time {
		calls++
		if calls == 1 {
			return start
		}
		return start.Add(deviceCodeLifetime)
	}
	_, err := client.pollDeviceCode(context.Background(), "device-1", "CODE-123", 0, now)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("pollDeviceCode error = %v, want expiry", err)
	}
}

func TestPollDeviceCodeContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.pollDeviceCode(ctx, "device-1", "CODE-123", time.Hour, time.Now)
	if err == nil {
		t.Fatal("pollDeviceCode succeeded after cancellation")
	}
}

func TestLoginDeviceCompletesExchangeAndSaves(t *testing.T) {
	store := &memoryStore{}
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			_, _ = io.WriteString(w, `{"device_auth_id":"device-1","user_code":"CODE-123","interval":0}`)
		case "/api/accounts/deviceauth/token":
			polls.Add(1)
			_, _ = io.WriteString(w, `{"authorization_code":"auth-code","code_verifier":"verifier","code_challenge":"challenge"}`)
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
			}
			if r.Form.Get("redirect_uri") != serverURLFromRequest(r)+"/deviceauth/callback" {
				t.Errorf("redirect_uri = %q", r.Form.Get("redirect_uri"))
			}
			writeTokenResponse(t, w, "device-access", "device-refresh", "", 3600)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &OAuthClient{Issuer: server.URL, HTTPClient: server.Client()}
	var output strings.Builder
	auth, err := client.LoginDevice(context.Background(), store, &output)
	if err != nil {
		t.Fatalf("LoginDevice: %v", err)
	}
	if auth.AccessToken != "device-access" || store.saves != 1 || polls.Load() != 1 {
		t.Errorf("auth = %#v, saves = %d, polls = %d", auth, store.saves, polls.Load())
	}
	if !strings.Contains(output.String(), server.URL+"/codex/device") || !strings.Contains(output.String(), "CODE-123") {
		t.Errorf("output = %q", output.String())
	}
}

func serverURLFromRequest(r *http.Request) string {
	return "http://" + r.Host
}

func TestFlexibleInt64RejectsInvalidValue(t *testing.T) {
	var value flexibleInt64
	if err := json.Unmarshal([]byte(`"2x"`), &value); err == nil {
		t.Fatal("invalid numeric string was accepted")
	}
}
