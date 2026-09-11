// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	_ "embed"
	"strings"
	"testing"
)

//go:embed testdata/nl_cases.txt
var nlCasesFile []byte

type nlCase struct {
	Name  string
	Input string
	Reply string
	Argv  []string
	Kind  string
}

func parseNLCases(t *testing.T, content string) []nlCase {
	t.Helper()
	var cases []nlCase
	var cur *nlCase
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-- ") {
			cases = append(cases, nlCase{Name: strings.TrimSpace(line[3:])})
			cur = &cases[len(cases)-1]
			continue
		}
		if cur == nil {
			t.Fatalf("catalog line before any case: %q", line)
		}
		switch {
		case strings.HasPrefix(line, "input: "):
			cur.Input = strings.TrimPrefix(line, "input: ")
		case strings.HasPrefix(line, "reply: "):
			cur.Reply = strings.TrimPrefix(line, "reply: ")
		case strings.HasPrefix(line, "argv: "):
			cur.Argv = strings.Fields(strings.TrimPrefix(line, "argv: "))
		case strings.HasPrefix(line, "kind: "):
			cur.Kind = strings.TrimPrefix(line, "kind: ")
		default:
			t.Fatalf("unknown catalog line: %q", line)
		}
	}
	return cases
}

func TestNaturalLanguageCatalog(t *testing.T) {
	cases := parseNLCases(t, string(nlCasesFile))
	if len(cases) < 8 {
		t.Fatalf("catalog has %d cases, want at least 8", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			if c.Input == "" || c.Reply == "" {
				t.Fatalf("case is missing input or reply: %+v", c)
			}
			p := newTestParser(&fakeLLM{call: intentCall(t, c.Reply)}, nil)
			r, err := p.Parse(context.Background(), c.Input, NewState())
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			switch {
			case len(c.Argv) > 0:
				if got := requireIntent(t, r); !equalArgs(got, c.Argv) {
					t.Fatalf("argv = %v, want %v", got, c.Argv)
				}
			case c.Kind == "clarify":
				requireClarify(t, r)
			case c.Kind == "reject":
				requireReject(t, r)
			default:
				t.Fatalf("case has neither argv nor kind: %+v", c)
			}
		})
	}
}
