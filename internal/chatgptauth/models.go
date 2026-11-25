// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package chatgptauth

import (
	"context"
	"errors"
	"net/http"
)

type Model struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Visibility  string `json:"visibility"`
}

func (c *Client) Models(ctx context.Context, auth *Auth) ([]Model, error) {
	if auth == nil || auth.AccessToken == "" || !auth.PlanEnabled() {
		return nil, errors.New("ChatGPT plan permission is required to list models")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Resource+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	var catalog struct {
		Models []Model `json:"models"`
	}
	if err := c.catalogRequest(req, auth, &catalog); err != nil {
		return nil, err
	}
	if catalog.Models == nil {
		return nil, errors.New("ChatGPT model catalog did not contain models")
	}
	var models []Model
	for _, m := range catalog.Models {
		if m.Visibility == "list" {
			if m.Slug == "" || m.DisplayName == "" {
				return nil, errors.New("invalid ChatGPT model catalog entry")
			}
			models = append(models, m)
		}
	}
	return models, nil
}
