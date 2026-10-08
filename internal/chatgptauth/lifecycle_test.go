// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoopbackLogin(t *testing.T) {
	for _, scenario := range []string{"dynamic", "return", "missing-id", "changed-id", "bad-state", "denied", "wrong-identity", "bad-signature", "disabled-plan", "bad-host"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			returning := scenario == "return" || scenario == "changed-id" || scenario == "wrong-identity" || scenario == "bad-signature"
			if returning {
				saveTest(t, s, testAuth())
			}
			previousBrowser := openBrowser
			defer func() { openBrowser = previousBrowser }()
			openBrowser = func(_ context.Context, local string) error {
				client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
				r, err := client.Get(local)
				if err != nil {
					return err
				}
				r.Body.Close()
				authorize, err := r.Location()
				if err != nil {
					return err
				}
				q := authorize.Query()
				f.claims["nonce"] = q.Get("nonce")
				redirect := q.Get("redirect_uri")
				u, _ := url.Parse(redirect)
				if u.Hostname() != "127.0.0.1" || u.Path != "/auth/callback" {
					t.Fatal(redirect)
				}
				if returning && q.Get("client_id") != "oaiapp_one" {
					t.Fatal(q)
				}
				if !returning && q.Get("client_id") != DynamicClient {
					t.Fatal(q)
				}
				host, err := os.ReadFile(filepath.Join(s.Dir, "host.json"))
				if err != nil || !bytes.Contains(host, []byte(q.Get("ext_agent_host_id"))) {
					t.Fatal("host not saved before authorization", err)
				}
				f.token = func(r *http.Request) any {
					if r.Form.Get("redirect_uri") != redirect || r.Form.Get("resource") != Resource || r.Form.Get("client_id") != "oaiapp_one" {
						t.Error("exchange mismatch")
					}
					if r.Form.Get("code_verifier") == "" {
						t.Error("missing PKCE")
					}
					b := f.tokens()
					if scenario == "bad-signature" {
						b["id_token"] = f.jwt() + "bad"
					}
					if scenario == "disabled-plan" {
						b["scope"] = "openid profile email offline_access"
					}
					return b
				}
				callback := url.Values{"state": {q.Get("state")}, "code": {"code"}, "client_id": {"oaiapp_one"}}
				switch scenario {
				case "missing-id":
					callback.Del("client_id")
				case "return":
					callback.Del("client_id")
				case "changed-id":
					callback.Set("client_id", "oaiapp_other")
				case "bad-state":
					callback.Set("state", "wrong")
				case "denied":
					callback.Set("error", "access_denied")
				case "wrong-identity":
					f.claims["sub"] = "different"
				}
				req, _ := http.NewRequest("GET", redirect+"?"+callback.Encode(), nil)
				if scenario == "bad-host" {
					req.Host = "evil.invalid"
					cancel()
				}
				r, err = client.Do(req)
				if err == nil {
					r.Body.Close()
				}
				return nil
			}
			var output bytes.Buffer
			auth, err := f.client.Login(ctx, s, "", false, false, false, &output)
			success := scenario == "dynamic" || scenario == "return" || scenario == "disabled-plan"
			if (err == nil) != success {
				t.Fatal(scenario, err)
			}
			if success {
				if auth.ClientID != "oaiapp_one" {
					t.Fatal(auth)
				}
				if scenario == "disabled-plan" {
					if auth.PlanEnabled() {
						t.Fatal("missing grant allowed")
					}
					if _, err := f.client.Credentials(ctx, s, ""); err == nil {
						t.Fatal("inference allowed without direct scope")
					}
				}
			} else if returning {
				old, err := s.Selected("")
				if err != nil || old.RefreshToken != "refresh" {
					t.Fatal("failed login replaced prior credentials", err)
				}
			}
			if strings.Contains(output.String(), "id_token_hint") || strings.Contains(output.String(), "access-new") {
				t.Fatal("credentials in output")
			}
		})
	}
}

func TestRefreshFailuresAndCancellation(t *testing.T) {
	for _, scenario := range []string{"terminal", "temporary", "cancel", "identity", "nonce", "new-id", "new-scope", "early"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			a.Nonce = "nonce"
			a.ExpiresAt = time.Now().Add(-time.Minute)
			if scenario == "early" {
				a.ExpiresAt = time.Now().Add(time.Minute)
				a.EarliestRefreshAt = time.Now().Add(30 * time.Second)
			}
			saveTest(t, s, a)
			if scenario == "terminal" || scenario == "temporary" || scenario == "cancel" {
				f.client.http = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
					if scenario == "cancel" {
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					status := 400
					code := "invalid_grant"
					if scenario == "temporary" {
						status = 503
						code = "server_error"
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"` + code + `"}`))}, nil
				})}
			} else {
				f.token = func(r *http.Request) any {
					b := f.tokens()
					switch scenario {
					case "identity":
						f.claims["sub"] = "other"
						b["id_token"] = f.jwt()
					case "nonce":
						f.claims["nonce"] = "other"
						b["id_token"] = f.jwt()
					case "new-scope":
						b["scope"] = "openid email"
					case "new-id":
						delete(f.claims, "nonce")
						b["id_token"] = f.jwt()
					}
					return b
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			if scenario != "cancel" {
				cancel()
				ctx = context.Background()
			} else {
				defer cancel()
			}
			result, err := f.client.Credentials(ctx, s, "")
			success := scenario == "new-id" || scenario == "early"
			if (err == nil) != success {
				t.Fatal(result, err)
			}
			saved, loadErr := s.Selected("")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if scenario == "terminal" && saved.AccessToken != "" {
				t.Fatal("terminal tokens retained")
			}
			if (scenario == "temporary" || scenario == "cancel" || scenario == "identity" || scenario == "nonce") && saved.RefreshToken != "refresh" {
				t.Fatal("old tokens lost")
			}
			if scenario == "early" && f.calls.Load() != 0 {
				t.Fatal("refreshed before earliest_refresh_at")
			}
			if scenario == "new-scope" && saved.PlanEnabled() {
				t.Fatal("new scope ignored")
			}
		})
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDiscoveryAndJWKSFailures(t *testing.T) {
	f := fixture(t)
	for _, body := range []string{`{}`, `{"issuer":"wrong"}`, `{"issuer":"https://auth.openai.com","jwks_uri":"https://evil.invalid/jwks","revocation_endpoint":"https://evil.invalid/revoke"}`} {
		f.client.keys = keyCache{}
		f.client.http = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
		if _, err := f.client.verify(context.Background(), f.jwt(), "oaiapp_one", "nonce"); err == nil {
			t.Fatal("bad discovery accepted")
		}
	}
	f.client.http = f.server.Client()
	f.client.keys = keyCache{keys: []jwk{{Kid: "key", Kty: "RSA", Alg: "RS256", N: "invalid!", E: "AQAB"}}, until: time.Now().Add(time.Hour)}
	if _, err := f.client.verify(context.Background(), f.jwt(), "oaiapp_one", "nonce"); err == nil {
		t.Fatal("bad JWK accepted")
	}
	f.client.keys = keyCache{keys: []jwk{{Kid: "key", Kty: "RSA", Alg: "RS256", N: base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()), E: ""}}, until: time.Now().Add(time.Hour)}
	if _, err := f.client.verify(context.Background(), f.jwt(), "oaiapp_one", "nonce"); err == nil {
		t.Fatal("bad exponent")
	}
}
func TestModelsCatalog(t *testing.T) {
	c := NewClient()
	a := testAuth()
	for _, body := range []string{`{"models":[{"slug":"second","display_name":"Second","visibility":"list"},{"slug":"hidden","display_name":"Hidden","visibility":"hide"},{"slug":"first","display_name":"First","visibility":"list"}]}`, `{"data":[]}`, `{"models":[{"visibility":"list"}]}`} {
		c.http = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != Resource+"/models" || r.Header.Get("Authorization") != "Bearer access" {
				t.Error("bad catalog request")
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		models, err := c.Models(context.Background(), &a)
		if strings.Contains(body, "second") {
			if err != nil || len(models) != 2 || models[0].Slug != "second" || models[1].Slug != "first" {
				t.Fatal(models, err)
			}
		} else if err == nil {
			t.Fatal("bad catalog accepted")
		}
	}
	if _, err := c.Models(context.Background(), nil); err == nil {
		t.Fatal("nil credentials accepted")
	}
}
func TestStoreAtomicFailureAndSelect(t *testing.T) {
	s := testStore(t)
	a := testAuth()
	saveTest(t, s, a)
	b := a
	b.ClientID = "oaiapp_two"
	saveTest(t, s, b)
	if err := s.Select(context.Background(), a.ClientID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Selected("")
	if got.ClientID != a.ClientID {
		t.Fatal(got)
	}
	if err := s.Select(context.Background(), "missing"); err == nil {
		t.Fatal("missing selection")
	}
	oldRename := renameFile
	renameFile = func(string, string) error { return errors.New("fixture rename failure") }
	defer func() { renameFile = oldRename }()
	if err := s.Clear(context.Background(), ""); err == nil {
		t.Fatal("rename failure swallowed")
	}
	got, _ = s.Selected("")
	if got.RefreshToken != "refresh" {
		t.Fatal("atomicity")
	}
	files, _ := filepath.Glob(filepath.Join(s.Dir, ".write-*"))
	if len(files) > 0 {
		t.Fatal(files)
	}
}
func TestTokenResponseValidation(t *testing.T) {
	scope := Scope
	valid := tokenResponse{Access: "a", Refresh: "r", ID: "i", Type: "Bearer", Expires: 3600, Scope: &scope}
	for _, early := range []string{`1790032532`, `"2020-01-01T00:00:00Z"`, `null`, `"invalid"`, `{}`, `999999999999`} {
		v := valid
		v.Earliest = json.RawMessage(early)
		a := Auth{}
		err := v.apply(&a, false)
		if (err == nil) != (early == `1790032532` || early == `"2020-01-01T00:00:00Z"` || early == `null`) {
			t.Fatal(early, err)
		}
	}
	missing := valid
	missing.ID = ""
	if err := missing.apply(&Auth{}, false); err == nil {
		t.Fatal("missing ID")
	}
	missing = valid
	missing.Scope = nil
	if err := missing.apply(&Auth{}, false); err == nil {
		t.Fatal("missing scope")
	}
	valid.Expires = 86401
	if err := valid.apply(&Auth{}, true); err == nil {
		t.Fatal("oversize expiry")
	}
}
func TestLogoutUnconfirmedRevocation(t *testing.T) {
	f := fixture(t)
	s := testStore(t)
	saveTest(t, s, testAuth())
	f.client.http = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("network unavailable") })}
	if err := f.client.Logout(context.Background(), s, ""); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatal(err)
	}
	a, _ := s.Selected("")
	if a.AccessToken != "" || a.IDToken != "" {
		t.Fatal("tokens retained")
	}
}
func TestCrossProcessSessionLock(t *testing.T) {
	if dir := os.Getenv("OCR_SIWC_TEST_LOCK_DIR"); dir != "" {
		s := Store{Dir: dir}
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		if err := s.withLock(ctx, func() error { return errors.New("unexpectedly acquired") }); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		return
	}
	s := testStore(t)
	err := s.withLock(context.Background(), func() error {
		cmd := exec.Command(os.Args[0], "-test.run=^TestCrossProcessSessionLock$")
		cmd.Env = append(os.Environ(), "OCR_SIWC_TEST_LOCK_DIR="+s.Dir)
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Log(string(b))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestHTTPFailureShape(t *testing.T) {
	c := NewClient()
	for _, body := range []string{`{"error":"invalid_refresh_token"}`, `{"error":"SECRET-ECHO"}`, `{"detail":"unavailable"}`, `invalid JSON`} {
		c.http = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 400, Header: http.Header{"X-Request-Id": {"fixture-request"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		err := c.post(context.Background(), Issuer+"/api/accounts/oauth/token", url.Values{}, nil)
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal(err)
		}
	}
}

type loginCancelWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *loginCancelWriter) Write(b []byte) (int, error) {
	n, err := w.Buffer.Write(b)
	if strings.Contains(string(b), "http://127.0.0.1:") {
		w.cancel()
	}
	return n, err
}
func TestLoopbackNoBrowserCancellation(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := loginCancelWriter{cancel: cancel}
	if _, err := NewClient().Login(ctx, s, "", false, true, false, &out); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "http://127.0.0.1:") || strings.Contains(out.String(), "auth.openai.com") {
		t.Fatal(out.String())
	}
}
func TestCallbackOneTimeAndWrongMethod(t *testing.T) {
	f := fixture(t)
	s := testStore(t)
	old := openBrowser
	defer func() { openBrowser = old }()
	openBrowser = func(_ context.Context, local string) error {
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		start, err := client.Get(local)
		if err != nil {
			return err
		}
		target, _ := start.Location()
		start.Body.Close()
		second, _ := client.Get(local)
		if second.StatusCode != 410 {
			t.Error(second.StatusCode)
		}
		second.Body.Close()
		q := target.Query()
		callback := q.Get("redirect_uri")
		req, _ := http.NewRequest("POST", callback, nil)
		bad, err := client.Do(req)
		if err != nil {
			return err
		}
		if bad.StatusCode != 403 {
			t.Error(bad.StatusCode)
		}
		bad.Body.Close()
		f.claims["nonce"] = q.Get("nonce")
		r, err := client.Get(callback + "?" + url.Values{"state": {q.Get("state")}, "code": {"code"}, "client_id": {"oaiapp_one"}}.Encode())
		if err == nil {
			r.Body.Close()
		}
		return err
	}
	if _, err := f.client.Login(context.Background(), s, "", false, false, false, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestCallbackMalformedQueryDoesNotExchangeCode(t *testing.T) {
	for _, suffix := range []string{"&broken=%zz", "&broken=a;b"} {
		t.Run(suffix, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			old := openBrowser
			defer func() { openBrowser = old }()
			openBrowser = func(_ context.Context, local string) error {
				q, err := localAuthorization(local)
				if err != nil {
					return err
				}
				callback := q.Get("redirect_uri") + "?" + url.Values{"state": {q.Get("state")}, "code": {"sensitive-fixture-code"}, "client_id": {"oaiapp_one"}}.Encode() + suffix
				r, err := http.Get(callback)
				if err != nil {
					return err
				}
				defer r.Body.Close()
				body, err := io.ReadAll(r.Body)
				if r.StatusCode != 400 || strings.Contains(string(body), "sensitive-fixture-code") || strings.Contains(string(body), q.Get("state")) {
					t.Error("malformed callback response exposed authorization parameters or did not fail")
				}
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := f.client.Login(ctx, s, "", false, false, false, io.Discard)
			if got != nil || err == nil || err.Error() != "invalid OAuth callback query" || f.calls.Load() != 0 {
				t.Fatal("malformed callback reached code exchange", err)
			}
			d, err := s.Snapshot()
			if err != nil || len(d.Accounts) != 0 {
				t.Fatal("malformed callback saved an account", err)
			}
		})
	}
}
