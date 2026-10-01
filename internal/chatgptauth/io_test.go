// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDefaultStoreAndInvalidStores(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := DefaultStore()
	if err != nil || !strings.HasSuffix(s.Dir, "auth/chatgpt") {
		t.Fatal(s, err)
	}
	t.Setenv("HOME", "")
	if _, err := DefaultStore(); err == nil {
		t.Fatal("empty HOME")
	}
	if _, err := (Store{}).Host(context.Background()); err == nil {
		t.Fatal("empty store dir")
	}
	bad := filepath.Join(t.TempDir(), "file")
	os.WriteFile(bad, []byte("x"), 0600)
	if err := (Store{Dir: bad}).withLock(context.Background(), func() error { return nil }); err == nil {
		t.Fatal("store is a file")
	}
	s = testStore(t)
	s.secure()
	if err := s.write("invalid", make(chan int)); err == nil {
		t.Fatal("marshal accepted channel")
	}
	if err := s.Select(context.Background(), ""); err == nil {
		t.Fatal("empty account")
	}
	if err := s.Clear(context.Background(), ""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for _, content := range []string{`{"accounts":[{"client_id":"dynamic_agent_client","issuer":"https://auth.openai.com","subject":"s"}]}`, `{"active":"missing","accounts":[]}`, `{"accounts":[{"client_id":"oaiapp","issuer":"other","subject":"s"}]}`} {
		os.WriteFile(filepath.Join(s.Dir, "accounts.json"), []byte(content), 0600)
		if _, err := s.Snapshot(); err == nil {
			t.Fatal("invalid registration accepted")
		}
	}
	os.Remove(filepath.Join(s.Dir, "accounts.json"))
	os.Mkdir(filepath.Join(s.Dir, "accounts.json"), 0700)
	if _, err := s.Selected(""); err == nil {
		t.Fatal("store path is directory")
	}
	os.Remove(filepath.Join(s.Dir, "accounts.json"))
	os.WriteFile(filepath.Join(s.Dir, "host.json"), []byte("bad"), 0600)
	if _, err := s.Host(context.Background()); err == nil {
		t.Fatal("invalid host JSON")
	}
}
func TestLogoutRevocationRetriesAndCancellation(t *testing.T) {
	for _, scenario := range []string{"discovery", "revoke", "permanent", "cancel", "signed-out"} {
		t.Run(scenario, func(t *testing.T) {
			f := fixture(t)
			s := testStore(t)
			a := testAuth()
			if scenario == "signed-out" {
				a.clear()
			}
			saveTest(t, s, a)
			baseTransport := f.client.http.Transport
			attempts := 0
			ctx := context.Background()
			var cancel context.CancelFunc
			if scenario == "cancel" {
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
			}
			f.client.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if scenario == "signed-out" {
					t.Error("signed-out revocation called")
				}
				if (scenario == "discovery" && strings.Contains(r.URL.Path, "openid")) || (scenario != "discovery" && r.URL.Path == "/revoke") {
					attempts++
					if scenario == "permanent" {
						return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":"invalid_client"}`)), Header: make(http.Header)}, nil
					}
					if attempts < 2 || scenario == "cancel" {
						return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{"error":"server_error"}`)), Header: make(http.Header)}, nil
					}
				}
				return baseTransport.RoundTrip(r)
			})
			err := f.client.Logout(ctx, s, "")
			if scenario == "cancel" || scenario == "permanent" {
				if err == nil {
					t.Fatal("unconfirmed revocation ignored")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			stored, _ := s.Selected("")
			if stored.AccessToken != "" || stored.IDToken != "" {
				t.Fatal("tokens retained")
			}
			if scenario == "discovery" || scenario == "revoke" {
				if attempts != 2 {
					t.Fatal(attempts)
				}
			}
		})
	}
}
func TestOAuthNoRedirects(t *testing.T) {
	c := NewClient()
	r := &http.Request{}
	if err := c.http.CheckRedirect(r, nil); err != http.ErrUseLastResponse {
		t.Fatal(err)
	}
}
func TestBrowserOpenerUsesExecutableAndWaits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixture")
	}
	dir := t.TempDir()
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	marker := filepath.Join(dir, "opened")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$1\" > '%s'\n", marker)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := openBrowserURL(context.Background(), "http://127.0.0.1/fixture"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(marker)
	if string(b) != "http://127.0.0.1/fixture" {
		t.Fatal(string(b))
	}
	os.Remove(filepath.Join(dir, name))
	if err := openBrowserURL(context.Background(), "http://127.0.0.1/fixture"); err == nil {
		t.Fatal("missing browser opener")
	}
}
