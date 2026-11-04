// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

type failedCatalogBody struct{}

func (failedCatalogBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failedCatalogBody) Close() error             { return nil }

func TestCatalogErrorBoundsAndShapes(t *testing.T) {
	a := testAuth()
	a.IDToken = "long-id-token"
	for _, tc := range []struct{ body, shape string }{
		{`[]`, "array"}, {`"unsafe access"`, "string"}, {`123`, "number"}, {`null`, "null"}, {`true`, "boolean"},
		{`{"other":"access"}`, "object"}, {`{"error":{},"detail":"route restriction"}`, "error_object"},
	} {
		e := catalogError(401, "req", []byte(tc.body), &a).(*APIError)
		if e.Shape != tc.shape || strings.Contains(e.Error(), a.AccessToken) {
			t.Fatal(e)
		}
		if strings.Contains(tc.body, "route restriction") && e.Detail != "route restriction" {
			t.Fatal(e)
		}
	}
	raw := "safe \x1b\n\r\t\x00 text access refresh long-id-token token='other secret' " + strings.Repeat("z", 1023) + "\u20ac"
	got := diagnosticText(raw, &a)
	if strings.ContainsAny(got, "\x1b\n\r\t\x00") || !utf8.ValidString(got) || len(got) > 1040 || !strings.HasSuffix(got, "... (truncated)") {
		t.Fatal("invalid bounded diagnostic")
	}
	for _, secret := range []string{"access", "refresh", "long-id-token", "other secret"} {
		if strings.Contains(got, secret) {
			t.Fatal("secret in bounded diagnostic")
		}
	}
}

func TestCatalogRequestFailurePaths(t *testing.T) {
	a := testAuth()
	for _, scenario := range []string{"network", "read", "oversized-error", "oversized-success", "invalid-json"} {
		t.Run(scenario, func(t *testing.T) {
			c := NewClient()
			c.http = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				if scenario == "network" {
					return nil, errors.New("network unavailable")
				}
				status := 200
				body := io.NopCloser(strings.NewReader("invalid-json"))
				switch scenario {
				case "read":
					body = failedCatalogBody{}
				case "oversized-error":
					status = 403
					body = io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1)))
				case "oversized-success":
					body = io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1)))
				}
				return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": {"req-bound"}}, Body: body}, nil
			})}
			_, err := c.Models(context.Background(), &a)
			if err == nil {
				t.Fatal("invalid response accepted")
			}
			if strings.HasPrefix(scenario, "oversized") {
				var api *APIError
				if !errors.As(err, &api) || api.Shape != "oversized" || api.RequestID != "req-bound" || len(err.Error()) > 2048 {
					t.Fatal(err)
				}
			}
		})
	}
}
