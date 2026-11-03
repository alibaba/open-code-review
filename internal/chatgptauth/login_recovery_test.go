// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
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
