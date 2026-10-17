// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// PluginReviewReport preserves a plugin's latest state for one review stage.
type PluginReviewReport struct {
	Plugin     string          `json:"plugin"`
	Group      string          `json:"group"`
	Stage      string          `json:"stage"`
	Complete   bool            `json:"complete"`
	TokenLimit int64           `json:"token_limit,omitempty"`
	Report     json.RawMessage `json:"report,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// RecordPluginReview flushes a stage report before execution proceeds, so the
// plugin's state remains inspectable if a later stage or the run fails.
func (sh *SessionHistory) RecordPluginReview(report PluginReviewReport) error {
	if sh == nil {
		return nil
	}
	sh.mu.Lock()
	p, initErr := sh.persist, sh.persistInitErr
	sh.mu.Unlock()
	if initErr != nil {
		return initErr
	}
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	uuid := generateUUID()
	data, err := json.Marshal(map[string]any{
		"uuid":          uuid,
		"parentUuid":    p.lastUUID,
		"type":          "plugin_review",
		"sessionId":     p.sessionID,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
		"review_plugin": report,
	})
	if err != nil {
		return fmt.Errorf("marshal plugin_review: %w", err)
	}
	if p.writer == nil {
		return fmt.Errorf("write plugin_review: session writer is unavailable")
	}
	if _, err := p.writer.Write(data); err != nil {
		return fmt.Errorf("write plugin_review: %w", err)
	}
	if err := p.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("write plugin_review: %w", err)
	}
	if err := p.writer.Flush(); err != nil {
		return fmt.Errorf("flush plugin_review: %w", err)
	}
	p.lastUUID = uuid
	return nil
}

// LoadPluginReviewReports returns saved reports in group order, preserving the
// order in which stages within each group were recorded.
func LoadPluginReviewReports(repoDir, sessionID string) ([]PluginReviewReport, error) {
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, err
	}
	var reports []PluginReviewReport
	if err := walkSessionFile(path, func(rec summaryRecord) {
		if rec.Type == "plugin_review" && rec.ReviewPlugin != nil {
			reports = append(reports, *rec.ReviewPlugin)
		}
	}); err != nil {
		return nil, err
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].Group < reports[j].Group })
	return reports, nil
}
