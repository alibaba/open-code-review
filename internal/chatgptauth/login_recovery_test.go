// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginIgnoresUnrelatedCallbacks(t *testing.T) {
	for _, query := range []string{"", "state=wrong", "state=STATE&state=STATE", "state=STATE&broken=%zz", "state=STATE", "state=STATE&code=code", "state=STATE&code=a&code=b", "state=STATE&error=denied&error=denied", "state=STATE&code=code&error=denied", "state=STATE&code=&error=access_denied", "state=STATE&code=code&error=&client_id=oaiapp_one"} {
		t.Run(query, func(t *testing.T) {
			f := fixture(t)
			previous := openBrowser
			defer func() { openBrowser = previous }()
			openBrowser = func(_ context.Context, local string) error {
				q, err := localAuthorization(local)
				if err != nil {
					return err
				}
				invalid := strings.ReplaceAll(query, "STATE", q.Get("state"))
				r, err := http.Get(q.Get("redirect_uri") + "?" + invalid)
				if err != nil {
					return err
				}
				r.Body.Close()
				if r.StatusCode != http.StatusBadRequest {
					return fmt.Errorf("invalid callback status: %d", r.StatusCode)
				}
				f.claims["nonce"] = q.Get("nonce")
				return sendCallback(q, "oaiapp_one")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			got, err := f.client.Login(ctx, testStore(t), "", false, false, false, io.Discard)
			if err != nil || got == nil || f.calls.Load() != 1 {
				t.Fatal("valid callback could not complete login", err)
			}
		})
	}
}

func TestLoginCompletesWhileBrowserOpenerIsRunning(t *testing.T) {
	f := fixture(t)
	previous := openBrowser
	defer func() { openBrowser = previous }()
	release, stopped := make(chan struct{}), make(chan struct{})
	defer func() { close(release); <-stopped }()
	openBrowser = func(_ context.Context, local string) error {
		defer close(stopped)
		q, err := localAuthorization(local)
		if err != nil {
			return err
		}
		f.claims["nonce"] = q.Get("nonce")
		if err := sendCallback(q, "oaiapp_one"); err != nil {
			return err
		}
		<-release
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := f.client.Login(ctx, testStore(t), "", false, false, false, io.Discard)
	if err != nil || got == nil || f.calls.Load() != 1 {
		t.Fatal("browser lifetime blocked login", err)
	}
	select {
	case <-stopped:
		t.Fatal("browser opener exited before login completed")
	default:
	}
}

func TestLoginAcceptsMatchingStateDenial(t *testing.T) {
	f := fixture(t)
	previous := openBrowser
	defer func() { openBrowser = previous }()
	openBrowser = func(_ context.Context, local string) error {
		q, err := localAuthorization(local)
		if err != nil {
			return err
		}
		r, err := http.Get(q.Get("redirect_uri") + "?" + url.Values{"state": {q.Get("state")}, "error": {"access_denied"}}.Encode())
		if err != nil {
			return err
		}
		return r.Body.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := f.client.Login(ctx, testStore(t), "", false, false, false, io.Discard)
	if err == nil || err.Error() != "ChatGPT authorization was declined" || f.calls.Load() != 0 {
		t.Fatal("matching denial was not consumed", err)
	}
}

func gatedCallbackServer(handler http.Handler, stall bool) *http.Server {
	var draining chan struct{}
	if !stall {
		draining = make(chan struct{})
	}
	server := newCallbackServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handler.ServeHTTP(w, req)
		if req.URL.Path == "/auth/callback" {
			select {
			case <-draining:
			case <-req.Context().Done():
			}
		}
	}))
	if draining != nil {
		server.RegisterOnShutdown(func() { close(draining) })
	}
	return server
}

func (f *oauthFixture) readLoginCallback(local string, denied bool) error {
	authorization, err := localAuthorization(local)
	if err != nil {
		return err
	}
	f.claims["nonce"] = authorization.Get("nonce")
	query := url.Values{"state": {authorization.Get("state")}, "code": {"code"}, "client_id": {"oaiapp_one"}}
	wantStatus, wantBody := http.StatusOK, "Authorization received. Return to OpenCodeReview to confirm sign-in.\n"
	if denied {
		query.Del("code")
		query.Set("error", "access_denied")
		wantStatus, wantBody = http.StatusBadRequest, "Sign-in could not be verified.\n"
	}
	browser := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 5 * time.Second}
	reply, err := browser.Get(authorization.Get("redirect_uri") + "?" + query.Encode())
	if err != nil {
		return err
	}
	defer reply.Body.Close()
	body, err := io.ReadAll(reply.Body)
	if err == nil && (reply.StatusCode != wantStatus || string(body) != wantBody) {
		return fmt.Errorf("callback returned %d %q; want %d %q", reply.StatusCode, body, wantStatus, wantBody)
	}
	return err
}

func TestLoginDrainsCallbackResponse(t *testing.T) {
	for _, scenario := range []string{"success", "denied", "exchange failure", "canceled", "stalled"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			exchangeErr := errors.New("exchange failed")
			if scenario == "exchange failure" || scenario == "canceled" {
				f.client.http.Transport = roundTrip(func(req *http.Request) (*http.Response, error) {
					if scenario == "canceled" {
						cancel()
						return nil, req.Context().Err()
					}
					return nil, exchangeErr
				})
			}

			previousBrowser := openBrowser
			defer func() { openBrowser = previousBrowser }()
			response := make(chan error, 1)
			openBrowser = func(_ context.Context, local string) error {
				response <- f.readLoginCallback(local, scenario == "denied")
				return nil
			}

			start := time.Now()
			var issued string
			auth, err := f.client.loginAttempt(ctx, "host", nil, nil, false, false, io.Discard, &issued, func(handler http.Handler) *http.Server {
				return gatedCallbackServer(handler, scenario == "stalled")
			})
			var browserErr error
			select {
			case browserErr = <-response:
			case <-time.After(6 * time.Second):
				t.Fatal("browser did not finish the callback request")
			}
			if scenario == "stalled" {
				var transportErr *url.Error
				if !errors.As(browserErr, &transportErr) || transportErr.Timeout() || time.Since(start) < time.Second {
					t.Fatalf("stalled callback was not forcibly closed after the grace period: %v", browserErr)
				}
			} else if browserErr != nil {
				t.Errorf("callback response: %v", browserErr)
			}
			switch scenario {
			case "success", "stalled":
				if err != nil || auth == nil || auth.ClientID != "oaiapp_one" {
					t.Errorf("expected successful login: %v", err)
				}
			case "denied":
				if auth != nil || err == nil || err.Error() != "ChatGPT authorization was declined" {
					t.Errorf("expected authorization denial: %v", err)
				}
			case "exchange failure":
				if auth != nil || !errors.Is(err, exchangeErr) {
					t.Errorf("expected exchange failure: %v", err)
				}
			case "canceled":
				if auth != nil || !errors.Is(err, context.Canceled) {
					t.Errorf("expected canceled login: %v", err)
				}
			}
		})
	}
}
