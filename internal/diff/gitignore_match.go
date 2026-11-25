// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"path"
	"strings"
)

func matchGitignoreEntry(name, pattern string, directory bool) bool {
	pattern, directoryOnly := strings.CutSuffix(pattern, "/")
	pattern, anchored := strings.CutPrefix(pattern, "/")
	if pattern == "" || strings.HasSuffix(pattern, "/") || directoryOnly && !directory {
		return false
	}
	if !anchored && !strings.Contains(pattern, "/") {
		name = path.Base(name)
	}
	if !strings.ContainsAny(pattern, `*?[\`) {
		return name == pattern
	}
	var retryPattern, retryName string
	for name != "" {
		component, tail, escapedSlash := splitIgnoreComponent(pattern)
		if strings.HasPrefix(component, "**") && strings.TrimLeft(component, "*") == "" {
			if tail == "" {
				return true
			}
			if escapedSlash {
				_, name, _ = strings.Cut(name, "/")
			}
			pattern, retryPattern, retryName = tail, tail, name
			continue
		}
		part, rest, _ := strings.Cut(name, "/")
		if matchIgnoreComponent(component, part) {
			pattern, name = tail, rest
			continue
		}
		if retryName == "" {
			return false
		}
		_, retryName, _ = strings.Cut(retryName, "/")
		pattern, name = retryPattern, retryName
	}
	return pattern == ""
}

func splitIgnoreComponent(pattern string) (component, tail string, escapedSlash bool) {
	if !strings.Contains(pattern, "/") {
		return pattern, "", false
	}
	for index := 0; index < len(pattern); index++ {
		switch pattern[index] {
		case '/':
			return pattern[:index], pattern[index+1:], false
		case '\\':
			if strings.HasPrefix(pattern[index:], `\/`) {
				return pattern[:index], pattern[index+2:], true
			}
			index++
		case '[':
			width, _ := matchIgnoreClass(pattern[index:], 0)
			if width == 0 {
				return pattern, "", false
			}
			index += width - 1
		}
	}
	return pattern, "", false
}

func matchIgnoreComponent(pattern, name string) bool {
	var retryPattern, retryName string
	for len(name) > 0 {
		width, matched := 1, false
		if len(pattern) > 0 {
			switch pattern[0] {
			case '*':
				pattern = strings.TrimLeft(pattern, "*")
				retryPattern, retryName = pattern, name
				continue
			case '?':
				matched = true
			case '\\':
				width, matched = 2, len(pattern) > 1 && pattern[1] == name[0]
			case '[':
				width, matched = matchIgnoreClass(pattern, name[0])
			default:
				matched = pattern[0] == name[0]
			}
		}
		if matched {
			pattern, name = pattern[width:], name[1:]
			continue
		}
		if len(retryName) == 0 {
			return false
		}
		retryName = retryName[1:]
		pattern, name = retryPattern, retryName
	}
	return strings.TrimLeft(pattern, "*") == ""
}

var ignoreClassRanges = map[string]string{
	"alnum": "AZaz09", "alpha": "AZaz", "blank": "  \t\t", "cntrl": "\x00\x1f\x7f\x7f",
	"digit": "09", "graph": "!~", "lower": "az", "print": " ~", "punct": "!/:@[\x60{~",
	"space": "  \t\n\r\r", "upper": "AZ", "xdigit": "AFaf09",
}

func matchIgnoreClass(pattern string, value byte) (int, bool) {
	index := 1
	negated := index < len(pattern) && (pattern[index] == '!' || pattern[index] == '^')
	if negated {
		index++
	}
	first := index
	previous := -1
	matched := false
	for index < len(pattern) {
		member := pattern[index]
		if member == ']' && index != first {
			return index + 1, matched != negated
		}
		index++
		switch {
		case member == '[' && strings.HasPrefix(pattern[index:], ":"):
			body, _, closed := strings.Cut(pattern[index+1:], "]")
			if name, named := strings.CutSuffix(body, ":"); closed && named {
				ranges, known := ignoreClassRanges[name]
				if !known {
					return 0, false
				}
				for offset := 0; offset < len(ranges); offset += 2 {
					matched = matched || ranges[offset] <= value && value <= ranges[offset+1]
				}
				index += len(name) + 3
				previous = -1
				continue
			}
		case member == '-' && previous >= 0 && index < len(pattern) && pattern[index] != ']':
			if pattern[index] == '\\' {
				index++
			}
			if index == len(pattern) {
				return 0, false
			}
			matched = matched || previous <= int(value) && value <= pattern[index]
			index++
			previous = -1
			continue
		}
		if member == '\\' {
			if index == len(pattern) {
				return 0, false
			}
			member = pattern[index]
			index++
		}
		matched = matched || value == member
		previous = int(member)
	}
	return 0, false
}
