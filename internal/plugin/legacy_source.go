// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package plugin

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Only the old local cache is used; reading installed plugins never fetches an index.
func migrateLegacySources(installed map[string]Installed, data json.RawMessage) {
	var markets map[string]struct {
		Plugins []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if json.Unmarshal(data, &markets) != nil {
		return
	}
	for id, item := range installed {
		sourceID, market, ok := strings.Cut(item.Source, "@")
		if !ok || sourceID != id || !validID(market) {
			continue
		}
		for _, entry := range markets[market].Plugins {
			if entry.ID != id || (!filepath.IsAbs(entry.Source) && !strings.HasPrefix(strings.ToLower(entry.Source), "https:")) {
				continue
			}
			source, err := canonicalSource(entry.Source)
			if err != nil {
				continue
			}
			item.Source, item.PluginID = source, id
			installed[id] = item
			break
		}
	}
}
