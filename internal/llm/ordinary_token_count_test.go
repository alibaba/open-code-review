// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llm

import (
	"strings"
	"testing"
)

func TestCountTokensTreatsSpecialMarkersAsText(t *testing.T) {
	encoder, err := defaultTokenizer.getOrLoad("cl100k_base")
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"empty":          "",
		"source":         "func main() {\n\tfmt.Println(\"hello\")\n}\n",
		"unicode":        "\u4e2d\u6587 \u043a\u043e\u0434 \u00e9 e\u0301 \U0001f600\r\n",
		"invalid_utf8":   string([]byte{0xff, 'a', 0xc3, 'b', 0x80}),
		"endoftext":      "<|endoftext|>",
		"fill_in_middle": "<|fim_prefix|>before<|fim_suffix|>after<|fim_middle|>middle",
		"endofprompt":    "<|endofprompt|>",
		"mixed":          "\u4e2d<|endoftext|>\U0001f600<|fim_prefix|><|fim_suffix|> tail",
		"repeated":       strings.Repeat("<|endoftext|>\n", 32),
	} {
		t.Run(name, func(t *testing.T) {
			// Review text may spell model control tokens, but none is an allowed
			// special token in the existing counting contract.
			want := len(encoder.Encode(text, nil, nil))
			if got := CountTokens(text); got != want {
				t.Fatalf("got %d tokens, want %d", got, want)
			}
			if got := CountTokensForModel(text, "gpt-4"); got != want {
				t.Fatalf("model count = %d, want %d", got, want)
			}
		})
	}
	if got := CountTokens("<|endoftext|>"); got <= 1 {
		t.Fatalf("literal marker counted as a single special token: %d", got)
	}
}

func BenchmarkCountTokensOrdinary(b *testing.B) {
	for name, text := range map[string]string{
		"source":       strings.Repeat("if value != nil { return value.String() }\n", 1024),
		"markers_1024": strings.Repeat("<|endoftext|>\n", 1024),
		"markers_4096": strings.Repeat("<|endoftext|>\n", 4096),
	} {
		b.Run(name, func(b *testing.B) {
			want := CountTokens(text)
			b.SetBytes(int64(len(text)))
			b.ReportAllocs()
			for b.Loop() {
				if got := CountTokens(text); got != want {
					b.Fatalf("got %d tokens, want %d", got, want)
				}
			}
		})
	}
}
