// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

type APIError struct {
	Status    int
	Code      string
	Param     string
	RequestID string
	Shape     string
	Detail    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ChatGPT API request failed: status=%d code=%q param=%q request_id=%q body_shape=%s detail=%q", e.Status, e.Code, e.Param, e.RequestID, e.Shape, e.Detail)
}

var apiSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[^\s,;"']+`),
	regexp.MustCompile(`(?i)\b(?:access_token|refresh_token|id_token|api_key|token|authorization)\s*[=:]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
}

func diagnosticText(text string, auth *Auth) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
				return -1
			}
			return r
		}, strings.ToValidUTF8(s, ""))
	}
	text = clean(text)
	for _, secret := range []string{auth.AccessToken, auth.RefreshToken, auth.IDToken} {
		if secret = clean(secret); secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	for _, pattern := range apiSecretPatterns {
		text = pattern.ReplaceAllString(text, "[REDACTED]")
	}
	if len(text) > 1024 {
		text = strings.ToValidUTF8(text[:1024], "") + "... (truncated)"
	}
	return text
}
func catalogError(status int, requestID string, body []byte, auth *Auth) error {
	e := &APIError{Status: status, RequestID: diagnosticText(requestID, auth), Shape: "non_json"}
	if !json.Valid(body) {
		return e
	}
	switch bytes.TrimSpace(body)[0] {
	case '[':
		e.Shape = "array"
	case '"':
		e.Shape = "string"
	case 'n':
		e.Shape = "null"
	case 't', 'f':
		e.Shape = "boolean"
	case '{':
		e.Shape = "object"
	default:
		e.Shape = "number"
	}
	if e.Shape != "object" {
		return e
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return e
	}
	readString := func(b json.RawMessage) string {
		var text string
		_ = json.Unmarshal(b, &text)
		return diagnosticText(text, auth)
	}
	if raw, ok := fields["error"]; ok {
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) == nil && nested != nil {
			e.Shape = "error_object"
			e.Code = readString(nested["code"])
			e.Param = readString(nested["param"])
			e.Detail = readString(nested["message"])
		} else {
			var text string
			if json.Unmarshal(raw, &text) == nil {
				e.Shape = "error_string"
				e.Detail = diagnosticText(text, auth)
			}
		}
	}
	if raw, ok := fields["detail"]; ok {
		if e.Shape == "object" {
			e.Shape = "detail"
		}
		if text := readString(raw); text != "" {
			if e.Detail != "" {
				e.Detail += "; "
			}
			e.Detail += text
		}
	}
	return e
}
func (c *Client) catalogRequest(req *http.Request, auth *Auth, target any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ChatGPT API transport failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return &APIError{Status: resp.StatusCode, RequestID: diagnosticText(resp.Header.Get("x-request-id"), auth), Shape: "oversized", Detail: "response exceeded size limit"}
	}
	if resp.StatusCode != http.StatusOK {
		return catalogError(resp.StatusCode, resp.Header.Get("x-request-id"), body, auth)
	}
	if json.Unmarshal(body, target) != nil {
		return fmt.Errorf("invalid ChatGPT API response JSON")
	}
	return nil
}
