// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package intent

import (
	"context"
	"strings"
)

// parseSlash parses a deterministic slash command. It never calls the LLM, so
// /review and /scan keep working while the parsing LLM is down.
func (p *Parser) parseSlash(ctx context.Context, text string, st *State) (Result, error) {
	fields := strings.Fields(text)
	cmd := fields[0]

	var action string
	switch cmd {
	case "/review":
		action = actionReview
	case "/scan":
		action = actionScan
	default:
		p.clearPending(st)
		return RejectResult("unknown command: "+cmd,
			"Supported commands are /review and /scan. Describe anything else in natural language."), nil
	}

	rest := fields[1:]
	s := newSlots(action)
	if canResumeSlash(st.Pending(), action, rest) {
		s = slotsFromState(st)
	}
	for _, tok := range rest {
		if strings.Contains(tok, "..") && !strings.HasPrefix(tok, "-") {
			p.clearPending(st)
			return RejectResult("range syntax "+tok+" is not supported",
				"Write /review --from <base> --to <head> instead."), nil
		}
	}

	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		name, inline, hasInline := splitFlag(tok)

		if action == actionReview && (name == "--from" || name == "--to" || name == "--commit") {
			value, ok := takeValue(rest, &i, inline, hasInline)
			if !ok {
				p.clearPending(st)
				return RejectResult("missing value for "+name, "Example: /review --from main --to feature"), nil
			}
			if strings.Contains(value, "..") {
				p.clearPending(st)
				return RejectResult("range syntax "+value+" is not supported",
					"Write /review --from <base> --to <head> instead."), nil
			}
			switch name {
			case "--from":
				if s.reviewType == reviewCommit || s.commit != "" {
					p.clearPending(st)
					return RejectResult("--commit cannot be combined with --from or --to",
						"Use either /review --commit <ref> or /review --from <base> --to <head>."), nil
				}
				s.from, s.reviewType = value, reviewRange
			case "--to":
				if s.reviewType == reviewCommit || s.commit != "" {
					p.clearPending(st)
					return RejectResult("--commit cannot be combined with --from or --to",
						"Use either /review --commit <ref> or /review --from <base> --to <head>."), nil
				}
				s.to, s.reviewType = value, reviewRange
			case "--commit":
				if s.reviewType == reviewRange || s.from != "" || s.to != "" {
					p.clearPending(st)
					return RejectResult("--commit cannot be combined with --from or --to",
						"Use either /review --commit <ref> or /review --from <base> --to <head>."), nil
				}
				s.commit, s.reviewType = value, reviewCommit
			}
			continue
		}

		if action == actionScan && name == "--path" {
			value, ok := takeValue(rest, &i, inline, hasInline)
			if !ok {
				p.clearPending(st)
				return RejectResult("missing value for --path", "Example: /scan --path internal/agent"), nil
			}
			for _, part := range strings.Split(value, ",") {
				if part = strings.TrimSpace(part); part != "" {
					s.paths = append(s.paths, part)
				}
			}
			continue
		}

		switch {
		case name == "--staged":
			p.clearPending(st)
			return RejectResult("--staged is not supported by the OCR CLI",
				"Use /review for the whole change set (staged, unstaged and untracked)."), nil
		case action == actionReview && name == "--path":
			p.clearPending(st)
			return RejectResult("--path belongs to /scan, not /review",
				"Use /scan --path <dir>, or /review for the whole change set."), nil
		case action == actionScan && (name == "--from" || name == "--to" || name == "--commit"):
			p.clearPending(st)
			return RejectResult(name+" is not a /scan flag",
				"Use /review "+name+" ... instead."), nil
		default:
			// Any other token is forwarded to the contract validator, which
			// owns the flag whitelist and enum checks. A bare value that
			// follows a value-taking flag lands here too, so --effort high
			// reaches validateExtra as the pair it expects.
			s.extra = append(s.extra, tok)
		}
	}

	if action == actionReview {
		return p.finalizeReview(ctx, st, s)
	}
	return p.finalizeScan(ctx, st, s)
}

// canResumeSlash permits only a direct answer to a pending range
// clarification. Every other slash command begins a new request so stale
// values cannot change its target.
func canResumeSlash(p *Pending, action string, rest []string) bool {
	if p == nil || p.Action != action || action != actionReview || p.ReviewType != reviewRange {
		return false
	}
	missing := make(map[string]bool, len(p.Missing))
	for _, name := range p.Missing {
		missing[name] = true
	}
	if len(missing) == 0 {
		return false
	}
	if len(rest) == 0 || len(rest) > 2 {
		return false
	}
	name, inline, hasInline := splitFlag(rest[0])
	if (name != "--from" && name != "--to") || !missing[strings.TrimPrefix(name, "--")] {
		return false
	}
	if hasInline {
		return len(rest) == 1 && inline != "" && !strings.HasPrefix(inline, "-")
	}
	return len(rest) == 2 && !strings.HasPrefix(rest[1], "-")
}

// splitFlag separates an inline --flag=value form. Non-flags return the token
// itself as name so callers can report it as a stray argument.
func splitFlag(tok string) (name, value string, hasValue bool) {
	if !strings.HasPrefix(tok, "-") {
		return tok, "", false
	}
	if idx := strings.IndexByte(tok, '='); idx >= 0 {
		return tok[:idx], tok[idx+1:], true
	}
	return tok, "", false
}

// takeValue reads the value of a flag, from the inline form or the next token.
// A missing value or a next token that looks like a flag reports failure
// instead of consuming a flag as a value.
func takeValue(rest []string, i *int, inline string, hasInline bool) (string, bool) {
	if hasInline {
		if inline == "" {
			return "", false
		}
		return inline, true
	}
	if *i+1 >= len(rest) {
		return "", false
	}
	next := rest[*i+1]
	if strings.HasPrefix(next, "-") {
		return "", false
	}
	*i++
	return next, true
}
