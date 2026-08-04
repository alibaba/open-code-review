package session

import (
	"encoding/json"

	"github.com/alibaba/open-code-review/internal/model"
)

// LoadComments replays one session's JSONL and returns every review comment
// recorded in it, in file-completion order. It mirrors resume replay
// semantics: a later checkpoint for the same fingerprint supersedes the
// earlier one, and a subsequent review_item_failed drops it. Comments that
// were persisted without a path inherit the record's file path.
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
		case "review_item_done", "review_item_reused":
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
			g := &group{comments: comments}
			order = append(order, g)
			if rec.Fingerprint != "" {
				byFingerprint[rec.Fingerprint] = g
			}
		case "review_item_failed":
			if g, ok := byFingerprint[rec.Fingerprint]; ok {
				g.comments = nil
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
