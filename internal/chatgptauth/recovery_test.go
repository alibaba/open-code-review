// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type cancelBody struct {
	io.Reader
	cancel context.CancelFunc
}

func (b cancelBody) Close() error { b.cancel(); return nil }

func TestRefreshCommitAfterCancellation(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "validated", false: "invalid"}[valid], func(t *testing.T) {
			s := testStore(t)
			a := testAuth()
			a.ExpiresAt = time.Now().Add(-time.Minute)
			saveTest(t, s, a)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := `{"access_token":"rotated-access","refresh_token":"rotated-refresh","token_type":"Bearer","expires_in":3600}`
			if !valid {
				body = `{"access_token":"rotated-access","token_type":"Bearer","expires_in":3600}`
			}
			c := NewClient()
			c.http = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: cancelBody{strings.NewReader(body), cancel}}, nil
			})}
			result, err := c.Credentials(ctx, s, "")
			if err == nil || result != nil {
				t.Fatal("canceled request returned credentials", err)
			}
			if valid && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			saved, err := s.Selected("")
			if err != nil {
				t.Fatal(err)
			}
			expected := "refresh"
			if valid {
				expected = "rotated-refresh"
			}
			if saved.RefreshToken != expected {
				t.Fatal("rotation persistence", saved.RefreshToken)
			}
			if valid {
				c.http.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
					t.Error("saved rotation triggered another network request")
					return nil, errors.New("unexpected network request")
				})
				next, err := c.Credentials(context.Background(), s, "")
				if err != nil || next == nil || next.AccessToken != "rotated-access" || next.RefreshToken != "rotated-refresh" {
					t.Fatal("saved rotation was not reusable", err)
				}
			}
		})
	}
}

func localAuthorization(local string) (url.Values, error) {
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get(local)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	u, err := r.Location()
	if err != nil {
		return nil, err
	}
	return u.Query(), nil
}
func sendCallback(q url.Values, id string) error {
	r, err := http.Get(q.Get("redirect_uri") + "?" + url.Values{"state": {q.Get("state")}, "code": {"code-" + q.Get("state")}, "client_id": {id}}.Encode())
	if err != nil {
		return err
	}
	return r.Body.Close()
}

func TestExplicitPlanConsent(t *testing.T) {
	for _, consent := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "enable"}[consent], func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			a.Scopes = strings.Fields("openid profile email offline_access resource.invoke")
			saveTest(t, s, a)
			old := openBrowser
			defer func() { openBrowser = old }()
			openBrowser = func(_ context.Context, local string) error {
				q, err := localAuthorization(local)
				if err != nil {
					return err
				}
				prompt := ""
				if consent {
					prompt = "consent"
				}
				if q.Get("prompt") != prompt || q.Get("client_id") != a.ClientID || q.Get("scope") != Scope || q.Get("resource") != Resource || q.Get("agent_name_hint") != "" {
					t.Error(q)
				}
				host, err := s.Host(context.Background())
				if err != nil || q.Get("ext_agent_host_id") != host {
					t.Error("host changed", err)
				}
				f.claims["nonce"] = q.Get("nonce")
				f.token = func(*http.Request) any {
					b := f.tokens()
					if !consent {
						b["scope"] = strings.Join(a.Scopes, " ")
					}
					return b
				}
				return sendCallback(q, a.ClientID)
			}
			got, err := f.client.Login(context.Background(), s, "", false, false, consent, io.Discard)
			if err != nil || got.PlanEnabled() != consent {
				t.Fatal(got, err)
			}
		})
	}
	s := testStore(t)
	if _, err := NewClient().Login(context.Background(), s, "", false, true, true, io.Discard); !errors.Is(err, ErrNotFound) {
		t.Fatal("consent without retained registration", err)
	}
	if _, err := NewClient().Login(context.Background(), s, "", true, true, true, io.Discard); err == nil {
		t.Fatal("consent with new registration")
	}
}

func TestInvalidGrantAuthorizationRecovery(t *testing.T) {
	for _, succeeds := range []bool{true, false} {
		t.Run(map[bool]string{true: "retry-success", false: "bounded-failure"}[succeeds], func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			a.ClientID = "previous"
			saveTest(t, s, a)
			old := openBrowser
			defer func() { openBrowser = old }()
			var first url.Values
			var count int
			openBrowser = func(_ context.Context, local string) error {
				q, err := localAuthorization(local)
				if err != nil {
					return err
				}
				count++
				if count == 1 {
					first = q
					if q.Get("client_id") != DynamicClient {
						t.Error(q)
					}
				} else {
					if q.Get("client_id") != "oaiapp_one" || q.Get("agent_name_hint") != "" || q.Get("id_token_hint") != "" {
						t.Error(q)
					}
					for _, key := range []string{"state", "nonce", "code_challenge"} {
						if q.Get(key) == first.Get(key) {
							t.Error("reused binding", key)
						}
					}
					if q.Get("ext_agent_host_id") != first.Get("ext_agent_host_id") || q.Get("scope") != Scope || q.Get("resource") != Resource {
						t.Error(q)
					}
				}
				active, _ := s.Selected("")
				if active.ClientID != "previous" || active.RefreshToken != "refresh" {
					t.Error("pending registration activated")
				}
				f.claims["nonce"] = q.Get("nonce")
				return sendCallback(q, "oaiapp_one")
			}
			transport := f.client.http.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			var exchanges int
			f.client.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/accounts/oauth/token" {
					exchanges++
					r.ParseForm()
					if r.Form.Get("client_id") != "oaiapp_one" {
						t.Error("lost issued client")
					}
					r.Body = io.NopCloser(strings.NewReader(r.Form.Encode()))
					if exchanges == 1 || !succeeds {
						return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`))}, nil
					}
				}
				return transport.RoundTrip(r)
			})
			_, err := f.client.Login(context.Background(), s, "", true, false, false, io.Discard)
			if (err == nil) != succeeds || count != 2 || exchanges != 2 {
				t.Fatal("unbounded or missing recovery", count, exchanges, err)
			}
			prior, _ := s.Selected("previous")
			if prior.RefreshToken != "refresh" {
				t.Fatal("previous account changed")
			}
			if !succeeds {
				active, _ := s.Selected("")
				if active.ClientID != "previous" {
					t.Fatal("failed retry activated")
				}
			}
		})
	}
}

func TestPendingLoginConcurrentAccountOperations(t *testing.T) {
	for _, scenario := range []string{"new-read-select-logout", "new-refresh", "return-logout", "return-refresh", "return-login"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			if strings.HasSuffix(scenario, "refresh") {
				a.ExpiresAt = time.Now().Add(-time.Minute)
			}
			saveTest(t, s, a)
			old := openBrowser
			defer func() { openBrowser = old }()
			pending := make(chan url.Values, 1)
			release := make(chan struct{})
			openBrowser = func(_ context.Context, local string) error {
				q, err := localAuthorization(local)
				if err != nil {
					return err
				}
				pending <- q
				<-release
				f.claims["nonce"] = q.Get("nonce")
				id := a.ClientID
				if strings.HasPrefix(scenario, "new") {
					id = "oaiapp_two"
					f.claims["aud"] = id
				}
				return sendCallback(q, id)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				_, err := f.client.Login(ctx, s, "", strings.HasPrefix(scenario, "new"), false, false, io.Discard)
				finished <- err
			}()
			select {
			case <-pending:
			case <-ctx.Done():
				t.Fatal("no pending login")
			}
			// Release the browser even on a failed contention assertion.
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			operationCtx, operationCancel := context.WithTimeout(context.Background(), time.Second)
			defer operationCancel()
			if strings.HasSuffix(scenario, "refresh") {
				f.token = func(*http.Request) any {
					return map[string]any{"access_token": "access-rotated", "refresh_token": "refresh-rotated", "token_type": "Bearer", "expires_in": 3600}
				}
				got, err := f.client.Credentials(operationCtx, s, a.ClientID)
				if err != nil || got.RefreshToken != "refresh-rotated" {
					t.Fatal("refresh blocked", err)
				}
				f.token = nil
			} else if scenario == "return-login" {
				openBrowser = func(_ context.Context, local string) error {
					q, err := localAuthorization(local)
					if err != nil {
						return err
					}
					f.claims["nonce"] = q.Get("nonce")
					return sendCallback(q, a.ClientID)
				}
				if _, err := f.client.Login(operationCtx, s, a.ClientID, false, false, false, io.Discard); err != nil {
					t.Fatal("concurrent login blocked", err)
				}
			} else {
				got, err := f.client.Credentials(operationCtx, s, a.ClientID)
				if err != nil || got.AccessToken != "access" {
					t.Fatal("credentials blocked", err)
				}
				if err := s.Select(operationCtx, a.ClientID); err != nil {
					t.Fatal("selection blocked", err)
				}
				if err := f.client.Logout(operationCtx, s, a.ClientID); err != nil {
					t.Fatal("logout blocked", err)
				}
			}
			close(release)
			err := <-finished
			returning := strings.HasPrefix(scenario, "return")
			if (err != nil) != returning {
				t.Fatal("commit revision check", err)
			}
			saved, err := s.Selected(a.ClientID)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case strings.HasSuffix(scenario, "refresh"):
				if saved.RefreshToken != "refresh-rotated" {
					t.Fatal("rotation overwritten")
				}
			case scenario == "return-login":
				if saved.RefreshToken != "refresh-new" {
					t.Fatal("login overwritten")
				}
			default:
				if saved.AccessToken != "" || saved.RefreshToken != "" {
					t.Fatal("logged-out account resurrected")
				}
			}
		})
	}
}

func TestModelsAPIDiagnostics(t *testing.T) {
	for _, tc := range []struct{ body, shape, code, param, detail string }{
		{`{"error":{"code":"subscription_sharing_user_not_eligible","param":"model","message":"workspace not eligible"}}`, "error_object", "subscription_sharing_user_not_eligible", "model", "workspace not eligible"},
		{`{"detail":"region restriction"}`, "detail", "", "", "region restriction"},
		{`{"error":{"code":"invalid_model","param":"model","message":"Bearer access token=refresh id secret-id sk-secret123 eyJhbGciOiJSUzI1NiJ9.payload.signature"}}`, "error_object", "invalid_model", "model", "[REDACTED]"},
		{`not-json access`, "non_json", "", "", ""},
		{`{"error":"not eligible"}`, "error_string", "", "", "not eligible"},
	} {
		t.Run(tc.shape+tc.code, func(t *testing.T) {
			c := NewClient()
			a := testAuth()
			a.IDToken = "secret-id"
			c.http = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 403, Header: http.Header{"X-Request-Id": {"req-catalog"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			_, err := c.Models(context.Background(), &a)
			if err == nil {
				t.Fatal("error swallowed")
			}
			var api *APIError
			if !errors.As(err, &api) || api.Status != 403 || api.Code != tc.code || api.Param != tc.param || api.Shape != tc.shape || api.RequestID != "req-catalog" || !strings.Contains(api.Detail, tc.detail) {
				t.Fatal(err)
			}
			for _, secret := range []string{"Bearer access", "token=refresh", "secret-id", "sk-secret123", "eyJhbGciOiJSUzI1NiJ9.payload.signature"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("credential in diagnostic")
				}
			}
		})
	}
}
