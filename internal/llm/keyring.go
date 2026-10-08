// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// keyRing holds every API key configured for one provider and the key new
// requests start from. It is shared by all requests of a client, so once a key
// hits its usage limit the rest of the run moves on instead of rediscovering
// the limit on every request.
type keyRing struct {
	mu   sync.Mutex
	keys []string
	cur  int
}

func (r *keyRing) current() (int, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cur, r.keys[r.cur]
}

// rotate moves past the key at from and returns the key now current. When a
// concurrent request already rotated away from it, the ring stays where that
// request left it rather than skipping a key nobody has tried yet.
func (r *keyRing) rotate(from int) (int, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur == from {
		r.cur = (from + 1) % len(r.keys)
	}
	return r.cur, r.keys[r.cur]
}

// isKeyLimitStatus reports whether a response says this key, rather than the
// request, cannot be served: 429 for rate and usage limits, 402 for an
// exhausted balance or quota.
func isKeyLimitStatus(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusPaymentRequired
}

// sdkMaxRetries is the retry budget every client gives its SDK. With
// api_keys, it also caps how many keys one logical request can reach.
const sdkMaxRetries = 5

// keyFailoverFor returns the failover middleware for cfg, or nil when cfg
// carries a single key and there is nothing to fail over to.
func keyFailoverFor(cfg ClientConfig, authHeader string) func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	if len(cfg.FallbackAPIKeys) == 0 {
		return nil
	}
	keys := make([]string, 0, len(cfg.FallbackAPIKeys)+1)
	keys = append(keys, cfg.APIKey)
	keys = append(keys, cfg.FallbackAPIKeys...)
	return keyFailoverMiddleware(keys, authHeader)
}

// keyFailoverMiddleware sends each HTTP attempt with the ring's current key.
// When the provider answers with a key limit status it rotates the ring and
// hands the response back to the SDK, whose retry loop sends the next attempt
// with the next key. Failover never re-sends a request itself, so the SDK's
// retry budget (sdkMaxRetries) still bounds every logical request.
//
// When the ring moves to another key, a Retry-After hint longer than
// keySwitchMaxWait is dropped. Such a hint says when the exhausted key resets
// (hours, for a subscription window), and the SDKs honour it without a cap, so
// the request would time out before a key that may be healthy is ever tried.
// The SDK then falls back to its own backoff. Shorter hints still apply.
//
// The SDKs do not retry 402, so it is marked retryable here. That is the one
// case where failover adds attempts: without it a 402 ends the request.
//
// It must be registered before the retry observer and raw capture, so both
// see the auth header this middleware sets.
//
// authHeader is the header the key travels in: "authorization" (sent as a
// Bearer token) or the name of a header carrying the bare key.
func keyFailoverMiddleware(keys []string, authHeader string) func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	ring := &keyRing{keys: keys}
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		idx, key := ring.current()
		setAuthKey(req, authHeader, key)
		resp, err := next(req)
		if err != nil || !isKeyLimitStatus(resp.StatusCode) {
			return resp, err
		}
		nextIdx, _ := ring.rotate(idx)
		if nextIdx != idx {
			if wait, ok := retryAfterHint(resp.Header); ok && wait > keySwitchMaxWait {
				resp.Header.Del("Retry-After")
				resp.Header.Del("Retry-After-Ms")
			}
		}
		if resp.StatusCode == http.StatusPaymentRequired {
			resp.Header.Set("x-should-retry", "true")
		}
		fmt.Fprintf(os.Stderr, "[ocr] WARNING: API key %d of %d hit a usage limit (HTTP %d); the next attempt uses key %d\n",
			idx+1, len(keys), resp.StatusCode, nextIdx+1)
		return resp, err
	}
}

// keySwitchMaxWait is the longest Retry-After hint honoured when the next
// attempt goes out on a different key. It matches the SDKs' own backoff
// ceiling.
const keySwitchMaxWait = 8 * time.Second

// retryAfterHint reads the delay the SDKs would take from h, with the same
// precedence: retry-after-ms, then Retry-After as seconds or an HTTP date.
func retryAfterHint(h http.Header) (time.Duration, bool) {
	if v := h.Get("Retry-After-Ms"); v != "" {
		if ms, err := strconv.ParseFloat(v, 64); err == nil {
			return time.Duration(ms * float64(time.Millisecond)), true
		}
	}
	v := h.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if s, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Duration(s * float64(time.Second)), true
	}
	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t), true
	}
	return 0, false
}

func setAuthKey(req *http.Request, authHeader, key string) {
	switch authHeader {
	case "", "authorization":
		req.Header.Set("Authorization", "Bearer "+key)
	default:
		req.Header.Set(authHeader, key)
	}
}
