// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package mcp

import (
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ValidateRemoteURL requires TLS except for explicit local development endpoints.
func ValidateRemoteURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("remote MCP requires an HTTPS URL without credentials or fragment")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		ip, err := netip.ParseAddr(u.Hostname())
		if strings.EqualFold(u.Hostname(), "localhost") || (err == nil && ip.Unmap().IsLoopback()) {
			return nil
		}
	}
	return fmt.Errorf("remote MCP requires HTTPS; HTTP is allowed only for localhost or loopback IP addresses")
}

func remoteRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	target := *req.URL
	target.Fragment, target.RawFragment = "", ""
	if err := ValidateRemoteURL(target.String()); err != nil {
		return err
	}
	first := via[0].URL
	// The transport reapplies configured headers, including custom credentials.
	// Keep them on the original origin even when net/http would drop Authorization.
	if req.URL.Scheme != first.Scheme || !strings.EqualFold(req.URL.Hostname(), first.Hostname()) || remotePort(req.URL) != remotePort(first) {
		return fmt.Errorf("remote MCP redirects must keep the original scheme, host and port")
	}
	return nil
}

func remotePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
