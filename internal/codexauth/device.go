// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package codexauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	deviceCodeLifetime        = 15 * time.Minute
	defaultDevicePollInterval = 5 * time.Second
	minimumDevicePollInterval = time.Second
)

type deviceCode struct {
	VerificationURL string
	UserCode        string
	DeviceAuthID    string
	Interval        time.Duration
}

type deviceCodeResponse struct {
	DeviceAuthID string        `json:"device_auth_id"`
	UserCode     string        `json:"user_code"`
	Interval     flexibleInt64 `json:"interval"`
}

type devicePollResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
	CodeChallenge     string `json:"code_challenge"`
}

type oauthErrorResponse struct {
	Error string `json:"error"`
}

type flexibleInt64 int64

func (v *flexibleInt64) UnmarshalJSON(data []byte) error {
	var number int64
	if err := json.Unmarshal(data, &number); err == nil {
		*v = flexibleInt64(number)
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return errors.New("interval must be a number or numeric string")
	}
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return errors.New("interval must be a number or numeric string")
	}
	*v = flexibleInt64(number)
	return nil
}

// LoginDevice completes the Codex device-code flow and saves the resulting
// credentials.
func (c *OAuthClient) LoginDevice(ctx context.Context, store CodexStore, out io.Writer) (*CodexAuth, error) {
	if store == nil {
		return nil, errors.New("Codex credential store is required")
	}
	if out == nil {
		out = io.Discard
	}
	code, err := c.requestDeviceCode(ctx)
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(out, "Open %s and enter code: %s\n", code.VerificationURL, code.UserCode)
	poll, err := c.pollDeviceCode(ctx, code.DeviceAuthID, code.UserCode, code.Interval, time.Now)
	if err != nil {
		return nil, err
	}
	auth, err := c.exchangeCode(
		ctx,
		poll.AuthorizationCode,
		c.issuer()+"/deviceauth/callback",
		poll.CodeVerifier,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("complete device authorization: %w", err)
	}
	if err := store.Save(auth); err != nil {
		return nil, fmt.Errorf("save Codex credentials: %w", err)
	}
	return auth, nil
}

func (c *OAuthClient) requestDeviceCode(ctx context.Context) (deviceCode, error) {
	body := strings.NewReader(fmt.Sprintf(`{"client_id":%q}`, ClientID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer()+"/api/accounts/deviceauth/usercode", body)
	if err != nil {
		return deviceCode{}, fmt.Errorf("create device-code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(req)
	if err != nil {
		return deviceCode{}, fmt.Errorf("request device code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, response.Body)
		return deviceCode{}, fmt.Errorf("device-code endpoint returned %s", response.Status)
	}
	var decoded deviceCodeResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&decoded); err != nil {
		return deviceCode{}, fmt.Errorf("decode device-code response: %w", err)
	}
	if decoded.DeviceAuthID == "" || decoded.UserCode == "" {
		return deviceCode{}, errors.New("device-code response was incomplete")
	}
	interval := defaultDevicePollInterval
	intervalSeconds := int64(decoded.Interval)
	if intervalSeconds > 0 {
		if intervalSeconds > int64(deviceCodeLifetime/time.Second) {
			interval = deviceCodeLifetime
		} else {
			interval = time.Duration(intervalSeconds) * time.Second
		}
	}
	if interval < minimumDevicePollInterval {
		interval = minimumDevicePollInterval
	}
	return deviceCode{
		VerificationURL: c.issuer() + "/codex/device",
		UserCode:        decoded.UserCode,
		DeviceAuthID:    decoded.DeviceAuthID,
		Interval:        interval,
	}, nil
}

func (c *OAuthClient) pollDeviceCode(
	ctx context.Context,
	deviceAuthID string,
	userCode string,
	interval time.Duration,
	now func() time.Time,
) (devicePollResponse, error) {
	if now == nil {
		now = time.Now
	}
	deadline := now().Add(deviceCodeLifetime)
	for {
		// Server-derived values: JSON-encode rather than %q so the wire form
		// stays valid whatever the deviceauth backend emits.
		payload, err := json.Marshal(map[string]string{
			"device_auth_id": deviceAuthID,
			"user_code":      userCode,
		})
		if err != nil {
			return devicePollResponse{}, fmt.Errorf("encode device-code poll request: %w", err)
		}
		body := bytes.NewReader(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer()+"/api/accounts/deviceauth/token", body)
		if err != nil {
			return devicePollResponse{}, fmt.Errorf("create device-code poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := c.httpClient().Do(req)
		if err != nil {
			return devicePollResponse{}, fmt.Errorf("poll device authorization: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		closeErr := response.Body.Close()
		if readErr != nil {
			return devicePollResponse{}, fmt.Errorf("read device authorization response: %w", readErr)
		}
		if closeErr != nil {
			return devicePollResponse{}, fmt.Errorf("close device authorization response: %w", closeErr)
		}
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			var completed devicePollResponse
			if err := json.Unmarshal(data, &completed); err != nil {
				return devicePollResponse{}, fmt.Errorf("decode device authorization response: %w", err)
			}
			if completed.AuthorizationCode == "" || completed.CodeVerifier == "" {
				return devicePollResponse{}, errors.New("device authorization response was incomplete")
			}
			return completed, nil
		}

		var oauthErr oauthErrorResponse
		_ = json.Unmarshal(data, &oauthErr)
		pending := response.StatusCode == http.StatusForbidden ||
			response.StatusCode == http.StatusNotFound ||
			oauthErr.Error == "authorization_pending" ||
			oauthErr.Error == "token_pending"
		if oauthErr.Error == "slow_down" {
			pending = true
			interval += 5 * time.Second
		}
		if !pending {
			return devicePollResponse{}, fmt.Errorf("device authorization endpoint returned %s", response.Status)
		}
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return devicePollResponse{}, errors.New("device authorization expired after 15 minutes")
		}
		sleep := interval
		expiresAfterSleep := sleep >= remaining
		if expiresAfterSleep {
			sleep = remaining
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return devicePollResponse{}, ctx.Err()
		case <-timer.C:
			if expiresAfterSleep {
				return devicePollResponse{}, errors.New("device authorization expired after 15 minutes")
			}
		}
	}
}
