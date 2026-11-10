// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

// Some OpenAI-compatible gateways forward a model's raw completion text
// verbatim as tool-call arguments. A model whose response format leaks control
// tokens after its JSON — observed with a harmony-format model emitting
// "<|start|><|channel|>final<|message|>..." immediately after an otherwise
// valid code_comment object — then produces arguments that are valid JSON
// followed by bytes json.Unmarshal rejects the whole string for. parseToolArgs
// recovers from that by locating the first balanced top-level JSON value and
// parsing only that span, so trailing non-JSON content no longer fails an
// otherwise-correct tool call.

// extractTopLevelJSON scans s for the first balanced top-level JSON object or
// array — the only two forms a tool call's arguments take — and returns the
// span it occupies. ok is false when s never opens one, or opens one that
// never balances, in which case the caller should fall back to parsing s whole
// so a genuinely malformed input still fails with its original error.
//
// A plain depth counter (rather than a stack of expected closers) is enough:
// well-formed JSON is always properly nested, so the point where depth returns
// to zero is exactly the end of the top-level value regardless of whether {
// and [ are tracked separately. A span this finds is not assumed valid on its
// own — the caller still unmarshals it and falls back on any error, which is
// what catches a "balanced" span that mismatches bracket types.
//
// Content inside JSON strings is skipped without inspecting it, so a brace or
// bracket quoted in prose (e.g. a comment's suggested code) never perturbs the
// depth count. Escaped quotes and backslashes are tracked byte by byte, which
// is safe for UTF-8 since every continuation byte is >= 0x80 and cannot be
// mistaken for a structural character.
func extractTopLevelJSON(s string) (span string, ok bool) {
	start := -1
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if start == -1 {
			if c == '{' || c == '[' {
				start = i
				depth = 1
			}
			continue
		}

		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}
