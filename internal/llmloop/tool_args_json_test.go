// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import "testing"

// TestParseToolArgs_TrailingContentAfterValidJSON is the reported bug: a
// harmony-format model (openai.gpt-oss-20b-1:0 on Bedrock) leaks control
// tokens right after its otherwise-valid code_comment JSON. Before this fix,
// json.Unmarshal rejected the whole string and the model retried the same
// finding, reworded, with no cap.
func TestParseToolArgs_TrailingContentAfterValidJSON(t *testing.T) {
	raw := `{"comments":[{"content":"issue","existing_code":"foo","path":"a.go"}]}` +
		"<|start|><|channel|>final<|message|>All done reviewing this file."

	args, err := parseToolArgs(raw)
	if err != nil {
		t.Fatalf("parseToolArgs() error = %v, want nil", err)
	}

	comments, ok := args["comments"].([]any)
	if !ok || len(comments) != 1 {
		t.Fatalf("args[\"comments\"] = %#v, want one-element array", args["comments"])
	}
	entry, ok := comments[0].(map[string]any)
	if !ok || entry["content"] != "issue" || entry["path"] != "a.go" {
		t.Errorf("comments[0] = %#v, want content=issue path=a.go", comments[0])
	}
}

// TestParseToolArgs_NoValidJSONStillErrors confirms the recovery path is a
// pure addition: a raw string with no balanced JSON value anywhere still
// returns the original parse error, unchanged from before this fix.
func TestParseToolArgs_NoValidJSONStillErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"prose with no braces", "I could not produce a tool call this round."},
		{"unbalanced object", `{"comments":`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := parseToolArgs(tt.raw)
			if err == nil {
				t.Fatalf("parseToolArgs(%q) = %#v, nil, want an error", tt.raw, args)
			}
			if args != nil {
				t.Errorf("parseToolArgs(%q) args = %#v, want nil on error", tt.raw, args)
			}
		})
	}
}

// TestExtractTopLevelJSON covers extractTopLevelJSON directly: quoted braces
// inside strings must not perturb the depth count, and escaped quotes must
// not end a string early.
func TestExtractTopLevelJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSpan string
		wantOK   bool
	}{
		{
			name:     "object with trailing garbage",
			input:    `{"a":1}garbage`,
			wantSpan: `{"a":1}`,
			wantOK:   true,
		},
		{
			name:     "array with trailing garbage",
			input:    `[1,2,3]<|end|>`,
			wantSpan: `[1,2,3]`,
			wantOK:   true,
		},
		{
			name:     "brace quoted inside a string does not affect depth",
			input:    `{"content":"looks like a { brace"}trailing`,
			wantSpan: `{"content":"looks like a { brace"}`,
			wantOK:   true,
		},
		{
			name:     "escaped quote does not end the string early",
			input:    `{"content":"a \"quoted\" word"}trailing`,
			wantSpan: `{"content":"a \"quoted\" word"}`,
			wantOK:   true,
		},
		{
			name:   "no opening brace or bracket",
			input:  `not json at all`,
			wantOK: false,
		},
		{
			name:   "opens but never balances",
			input:  `{"a":1`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span, ok := extractTopLevelJSON(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("extractTopLevelJSON(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && span != tt.wantSpan {
				t.Errorf("extractTopLevelJSON(%q) span = %q, want %q", tt.input, span, tt.wantSpan)
			}
		})
	}
}
