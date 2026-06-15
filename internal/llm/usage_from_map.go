package llm

import "encoding/json"

// UsageFromMap extracts token usage from a loosely-typed map (e.g. Cursor turn-ended usage).
func UsageFromMap(m map[string]any) *UsageInfo {
	if len(m) == 0 {
		return nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	if ui := resolveUsage(raw); ui != nil {
		return ui
	}
	raw, err = json.Marshal(map[string]any{"usage": m})
	if err != nil {
		return nil
	}
	return resolveUsage(raw)
}
