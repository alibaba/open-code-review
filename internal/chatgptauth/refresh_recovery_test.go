// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRefreshRecoveryAfterVerificationFailure(t *testing.T) {
	for _, scenario := range []string{"healthy_id_token", "no_id_token", "jwks_unavailable", "jwks_missing_key", "cancel_after_token_response"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			a.ExpiresAt = time.Now().Add(-time.Minute)
			saveTest(t, s, a)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rotated, recoverNetwork := false, false
			refreshCalls, rejectedOld, failedJWKS := 0, 0, 0
			f.client.http = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				if err := r.Context().Err(); err != nil {
					return nil, err
				}
				if r.URL.Path == "/api/accounts/oauth/token" {
					refreshCalls++
					if err := r.ParseForm(); err != nil {
						return nil, err
					}
					if rotated && r.Form.Get("refresh_token") == a.RefreshToken {
						rejectedOld++
						return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant"}`))}, nil
					}
					rotated = true
					tokens := f.tokens()
					if scenario == "no_id_token" {
						delete(tokens, "id_token")
					}
					body, err := json.Marshal(tokens)
					if err != nil {
						return nil, err
					}
					var reader io.ReadCloser = io.NopCloser(strings.NewReader(string(body)))
					if scenario == "cancel_after_token_response" && !recoverNetwork {
						reader = cancelBody{Reader: strings.NewReader(string(body)), cancel: cancel}
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: reader}, nil
				}
				if r.URL.Path == "/.well-known/jwks.json" && scenario == "jwks_unavailable" && !recoverNetwork {
					failedJWKS++
					return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"temporarily_unavailable"}`))}, nil
				}
				if r.URL.Path == "/.well-known/jwks.json" && scenario == "jwks_missing_key" && !recoverNetwork {
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"keys":[]}`))}, nil
				}
				return http.DefaultTransport.RoundTrip(r)
			})}
			first, firstErr := f.client.Credentials(ctx, s, "")
			needsRecovery := scenario == "jwks_unavailable" || scenario == "jwks_missing_key" || scenario == "cancel_after_token_response"
			if needsRecovery {
				if firstErr == nil || first != nil || !rotated {
					t.Fatalf("fault did not occur after rotation: rotated=%t err=%v", rotated, firstErr)
				}
				if scenario == "jwks_unavailable" && failedJWKS != 1 {
					t.Fatalf("expected one JWKS fault, got %d", failedJWKS)
				}
				if scenario == "cancel_after_token_response" && !errors.Is(firstErr, context.Canceled) {
					t.Fatalf("expected canceled context, got %v", firstErr)
				}
			} else if firstErr != nil {
				t.Fatal(firstErr)
			}
			saved, err := s.Selected("")
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("first_error=%v stored_old_refresh=%t", firstErr, saved.RefreshToken == a.RefreshToken)
			if needsRecovery {
				if saved.PendingRefresh == nil || saved.PendingRefresh.Response.Refresh != "refresh-new" || saved.AccessToken != a.AccessToken {
					t.Fatal("replacement was lost or activated before verification")
				}
			}
			recoverNetwork = true
			restarted := NewClient()
			restarted.base, restarted.http = f.client.base, f.client.http
			next, nextErr := restarted.Credentials(context.Background(), s, "")
			after, err := s.Selected("")
			if err != nil {
				t.Fatal(err)
			}
			if needsRecovery && next != nil && !next.ExpiresAt.Equal(saved.PendingRefresh.ReceivedAt.Add(time.Hour)) {
				t.Fatal("recovery extended the original token lifetime")
			}
			t.Logf("refresh_calls=%d rejected_old=%d next_error=%v tokens_cleared=%t", refreshCalls, rejectedOld, nextErr, after.AccessToken == "" && after.RefreshToken == "")
			if nextErr != nil || next == nil || next.AccessToken != "access-new" || next.PendingRefresh != nil || refreshCalls != 1 || rejectedOld != 0 {
				t.Fatalf("refresh must recover after the temporary validation fault without replaying the consumed token: %v", nextErr)
			}
		})
	}
}

func TestPendingRefreshRejectsUnverifiedIdentity(t *testing.T) {
	for _, invalid := range []string{"malformed", "signature", "subject", "nonce", "expired_at_receipt"} {
		t.Run(invalid, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			if invalid == "subject" {
				f.claims["sub"] = "another-account"
			}
			if invalid == "nonce" {
				f.claims["nonce"] = "another-login"
			}
			if invalid == "expired_at_receipt" {
				f.claims["exp"] = time.Now().Add(-time.Minute).Unix()
			}
			var response tokenResponse
			b, err := json.Marshal(f.tokens())
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &response); err != nil {
				t.Fatal(err)
			}
			if invalid == "signature" {
				response.ID += "invalid"
			}
			if invalid == "malformed" {
				response.ID = "malformed"
			}
			a.PendingRefresh = &pendingRefresh{Response: response, ReceivedAt: time.Now()}
			saveTest(t, s, a)
			got, err := f.client.Credentials(context.Background(), s, "")
			if err == nil || got != nil || f.calls.Load() != 0 {
				t.Fatal("unverified pending credentials were used", err)
			}
			wantRecovery := "ocr auth login --provider chatgpt --account " + a.ClientID
			if !strings.Contains(err.Error(), wantRecovery) {
				t.Fatalf("permanent identity failure needs sign-in guidance: %v", err)
			}
			firstError := err.Error()
			saved, err := s.Selected("")
			if err != nil || saved.AccessToken != a.AccessToken || saved.RefreshToken != a.RefreshToken || saved.PendingRefresh == nil || saved.PendingRefresh.IdentityError == "" {
				t.Fatal("active identity changed", err)
			}
			restarted := NewClient()
			requests := 0
			restarted.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				requests++
				return nil, errors.New("unexpected network request")
			})
			if got, err := restarted.Credentials(context.Background(), s, ""); got != nil || err == nil || err.Error() != firstError || requests != 0 {
				t.Fatal("permanent failure was retried after restart", requests, err)
			}

			f.claims["sub"], f.claims["exp"] = a.Subject, time.Now().Add(time.Hour).Unix()
			previous := openBrowser
			defer func() { openBrowser = previous }()
			openBrowser = func(_ context.Context, local string) error {
				q, err := localAuthorization(local)
				if err != nil {
					return err
				}
				f.claims["nonce"] = q.Get("nonce")
				return sendCallback(q, a.ClientID)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, err := f.client.Login(ctx, s, "", false, false, false, io.Discard); err != nil {
				t.Fatal("could not sign in after permanent refresh failure", err)
			}
			if got, err := f.client.Credentials(ctx, s, ""); err != nil || got == nil || got.PendingRefresh != nil || got.AccessToken != "access-new" {
				t.Fatal("sign-in did not restore usable credentials", err)
			}
		})
	}
}

func TestPendingRefreshLogoutAndClear(t *testing.T) {
	for _, logout := range []bool{true, false} {
		t.Run(map[bool]string{true: "logout", false: "clear"}[logout], func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			a.PendingRefresh = &pendingRefresh{Response: tokenResponse{Refresh: "pending-refresh"}, ReceivedAt: time.Now(), IdentityError: "invalid ID token"}
			saveTest(t, s, a)
			transport := f.client.http.Transport
			revoked := false
			f.client.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/revoke" {
					if err := r.ParseForm(); err != nil {
						return nil, err
					}
					if r.Form.Get("token") != "pending-refresh" {
						t.Error("revocation used the consumed token")
					}
					revoked = true
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				return transport.RoundTrip(r)
			})
			var err error
			if logout {
				err = f.client.Logout(context.Background(), s, "")
			} else {
				err = s.Clear(context.Background(), "")
			}
			if err != nil {
				t.Fatal(err)
			}
			saved, err := s.Selected("")
			if err != nil || saved.PendingRefresh != nil || saved.AccessToken != "" || saved.RefreshToken != "" || revoked != logout {
				t.Fatal("pending credentials survived clearing", err)
			}
		})
	}
}

func TestPendingRefreshRecoversAfterIDAndAccessExpiry(t *testing.T) {
	f := fixture(t)
	s := testStore(t)
	a := testAuth()
	received := time.Now().Add(-2 * time.Minute)
	f.claims["iat"] = received.Add(-time.Minute).Unix()
	f.claims["exp"] = received.Add(time.Minute).Unix()
	a.PendingRefresh = &pendingRefresh{Response: tokenResponse{Access: "expired-access", Refresh: "refresh-new", ID: f.jwt(), Type: "Bearer", Expires: 60}, ReceivedAt: received}
	saveTest(t, s, a)
	f.token = func(r *http.Request) any {
		if r.Form.Get("refresh_token") != "refresh-new" {
			t.Error("replayed consumed refresh token")
		}
		saved, err := s.Selected("")
		if err != nil || saved.PendingRefresh != nil || saved.ExpiresAt.After(time.Now()) {
			t.Error("expired pending token was not committed with its original expiry", err)
		}
		f.claims["iat"] = time.Now().Unix()
		f.claims["exp"] = time.Now().Add(time.Hour).Unix()
		return f.tokens()
	}
	if got, err := f.client.Credentials(context.Background(), s, ""); err != nil || got == nil || got.AccessToken != "access-new" || f.calls.Load() != 1 {
		t.Fatal("verified replacement could not refresh again", err)
	}
}

func TestCredentialsRefreshWindow(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "pending"}[pending], func(t *testing.T) {
			for _, tc := range []struct {
				name              string
				remaining         time.Duration
				earliest          time.Duration
				refreshCalls      int32
				refreshedLifetime int
			}{
				{name: "outside_window", remaining: 10 * time.Minute},
				{name: "two_minutes", remaining: 2 * time.Minute, refreshCalls: 1},
				{name: "seconds_remaining", remaining: 15 * time.Second, refreshCalls: 1},
				{name: "before_earliest", remaining: 2 * time.Minute, earliest: time.Minute},
				{name: "after_earliest", remaining: 2 * time.Minute, earliest: -time.Minute, refreshCalls: 1},
				{name: "short_lived_replacement", remaining: 2 * time.Minute, refreshCalls: 1, refreshedLifetime: 120},
			} {
				t.Run(tc.name, func(t *testing.T) {
					f := fixture(t)
					s := testStore(t)
					a := testAuth()
					now := time.Now()
					expiry := now.Add(tc.remaining)
					a.ExpiresAt = expiry
					if tc.earliest != 0 {
						a.EarliestRefreshAt = now.Add(tc.earliest).Truncate(time.Second)
					}
					wantAccess, wantRefresh := a.AccessToken, a.RefreshToken
					if pending {
						received := now.Add(-10 * time.Minute)
						f.claims["iat"] = received.Unix()
						response := tokenResponse{Access: "access-pending", Refresh: "refresh-pending", ID: f.jwt(), Type: "Bearer", Expires: int64(expiry.Sub(received) / time.Second)}
						if tc.earliest != 0 {
							response.Earliest, _ = json.Marshal(a.EarliestRefreshAt.Unix())
						}
						a.PendingRefresh = &pendingRefresh{Response: response, ReceivedAt: received}
						a.ExpiresAt = now.Add(-time.Minute)
						wantAccess, wantRefresh = response.Access, response.Refresh
					}
					saveTest(t, s, a)
					f.token = func(r *http.Request) any {
						if r.Form.Get("refresh_token") != wantRefresh {
							t.Error("refresh did not use the latest verified token")
						}
						stored, err := s.Selected("")
						if err != nil || stored.PendingRefresh != nil || stored.AccessToken != wantAccess || !stored.ExpiresAt.Equal(expiry) {
							t.Error("credentials were not persisted before refreshing", err)
						}
						tokens := f.tokens()
						if tc.refreshedLifetime != 0 {
							tokens["expires_in"] = tc.refreshedLifetime
						}
						return tokens
					}
					got, err := f.client.Credentials(context.Background(), s, "")
					if err != nil || got == nil {
						t.Fatal("credentials unavailable", err)
					}
					if calls := f.calls.Load(); calls != tc.refreshCalls {
						t.Fatalf("refresh calls = %d, want %d", calls, tc.refreshCalls)
					}
					if tc.refreshCalls > 0 {
						wantAccess, wantRefresh = "access-new", "refresh-new"
					} else if !got.ExpiresAt.Equal(expiry) || !got.EarliestRefreshAt.Equal(a.EarliestRefreshAt) {
						t.Fatal("recovery changed the token refresh times")
					}
					if got.AccessToken != wantAccess || got.RefreshToken != wantRefresh || got.PendingRefresh != nil {
						t.Fatal("unexpected credentials returned")
					}
					stored, err := s.Selected("")
					if err != nil || stored.AccessToken != got.AccessToken || stored.RefreshToken != got.RefreshToken || stored.PendingRefresh != nil {
						t.Fatal("returned credentials were not persisted", err)
					}
				})
			}
		})
	}
}
