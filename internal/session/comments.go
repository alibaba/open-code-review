// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"encoding/json"

	"github.com/alibaba/open-code-review/internal/model"
)

// LoadComments replays one session's JSONL and returns every review comment
// recorded in it, in review item order. A later record for the same
// fingerprint replaces earlier comments, including an empty failed record
// that retracts them. Comments without a path inherit the record's file path.
func LoadComments(repoDir, sessionID string) ([]model.LlmComment, error) {
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, err
	}
	type group struct {
		comments []model.LlmComment
	}
	var order []*group
	byFingerprint := map[string]*group{}
	err = walkSessionFile(path, func(rec summaryRecord) {
		switch rec.Type {
		case "review_item_done", "review_item_reused", "review_item_failed":
			var comments []model.LlmComment
			if len(rec.Comments) > 0 {
				if err := json.Unmarshal(rec.Comments, &comments); err != nil {
					return
				}
			}
			filePath := rec.FilePath
			if filePath == "" {
				filePath = rec.NewPath
			}
			for i := range comments {
				if comments[i].Path == "" {
					comments[i].Path = filePath
				}
			}
			if rec.Fingerprint != "" {
				if g, ok := byFingerprint[rec.Fingerprint]; ok {
					g.comments = comments
					return
				}
			}
			if rec.Type == "review_item_failed" && len(comments) == 0 {
				return
			}
			g := &group{comments: comments}
			order = append(order, g)
			if rec.Fingerprint != "" {
				byFingerprint[rec.Fingerprint] = g
			}
		}
	})
	if err != nil {
		return nil, err
	}
	var out []model.LlmComment
	for _, g := range order {
		out = append(out, g.comments...)
	}
	return out, nil
}
