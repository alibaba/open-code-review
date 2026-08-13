// Package hashline implements content-hash line anchors for comment
// localization, adapted from the hashline protocol
// (github.com/RimuruW/pi-hashline-edit, itself adapted from oh-my-pi).
//
// Every line of a file gets a short hash computed from the line and its
// immediate neighbors (prev + "\0" + curr + "\0" + next). An anchor
// "LINE#HASH" therefore carries both a position (line number, the primary
// key) and a checksum (the hash, a verification factor). Identical lines in
// different contexts get different hashes, so anchors are unambiguous even
// for repeated code.
package hashline

import (
	"regexp"
	"strconv"
	"strings"
)

// HashLen is the number of hash characters per anchor (one nibble each).
const HashLen = 2

// NibbleStr is the 16-character hash alphabet from pi-hashline-edit.
// It excludes most hex digits, digit-lookalikes and vowels.
const NibbleStr = "ZPMQVRWSNKTXJBYH"

// anchorRe matches "12#KT" or a range "12#KT-18#MQ".
var anchorRe = regexp.MustCompile(
	`^\s*(\d+)#([` + NibbleStr + `]{2,4})(?:\s*-\s*(\d+)#([` + NibbleStr + `]{2,4}))?\s*$`)

// ─── xxh32 ──────────────────────────────────────────────────────────────

const (
	prime1 uint32 = 2654435761
	prime2 uint32 = 2246822519
	prime3 uint32 = 3266489917
	prime4 uint32 = 668265263
	prime5 uint32 = 374761393
)

func rotl32(x uint32, r uint) uint32 { return x<<r | x>>(32-r) }

func round32(acc, input uint32) uint32 {
	acc += input * prime2
	acc = rotl32(acc, 13)
	acc *= prime1
	return acc
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// XXH32 computes the xxHash32 of input with the given seed.
func XXH32(input []byte, seed uint32) uint32 {
	n := len(input)
	var h uint32
	i := 0
	if n >= 16 {
		v1 := seed + prime1 + prime2
		v2 := seed + prime2
		v3 := seed
		v4 := seed - prime1
		for ; i <= n-16; i += 16 {
			v1 = round32(v1, le32(input[i:]))
			v2 = round32(v2, le32(input[i+4:]))
			v3 = round32(v3, le32(input[i+8:]))
			v4 = round32(v4, le32(input[i+12:]))
		}
		h = rotl32(v1, 1) + rotl32(v2, 7) + rotl32(v3, 12) + rotl32(v4, 18)
	} else {
		h = seed + prime5
	}
	h += uint32(n)
	for ; i <= n-4; i += 4 {
		h += le32(input[i:]) * prime3
		h = rotl32(h, 17) * prime4
	}
	for ; i < n; i++ {
		h += uint32(input[i]) * prime5
		h = rotl32(h, 11) * prime1
	}
	h ^= h >> 15
	h *= prime2
	h ^= h >> 13
	h *= prime3
	h ^= h >> 16
	return h
}

// ─── Line hashing ───────────────────────────────────────────────────────

// NormalizeHashInput normalizes a line before hashing: strip \r, trim right.
func NormalizeHashInput(line string) string {
	return strings.TrimRight(strings.ReplaceAll(line, "\r", ""), " \t\n\v\f")
}

// ComputeHashFromContext computes a HashLen-char hash from a line and its
// normalized neighbors. Neighbors outside the file use "".
func ComputeHashFromContext(prev, curr, next string) string {
	h := XXH32([]byte(prev+"\x00"+curr+"\x00"+next), 0)
	var b strings.Builder
	for i := HashLen - 1; i >= 0; i-- {
		b.WriteByte(NibbleStr[(h>>(uint(i)*4))&0x0f])
	}
	return b.String()
}

// ComputeLineHash computes the anchor hash for fileLines[index] (0-based).
func ComputeLineHash(fileLines []string, index int) string {
	prev, next := "", ""
	if index > 0 {
		prev = NormalizeHashInput(fileLines[index-1])
	}
	if index < len(fileLines)-1 {
		next = NormalizeHashInput(fileLines[index+1])
	}
	return ComputeHashFromContext(prev, NormalizeHashInput(fileLines[index]), next)
}

// FormatAnchor renders "LINE#HASH" for fileLines[index] (0-based index,
// 1-based rendered line number).
func FormatAnchor(fileLines []string, index int) string {
	return strconv.Itoa(index+1) + "#" + ComputeLineHash(fileLines, index)
}

// ─── Anchor parsing & resolution ────────────────────────────────────────

// Anchor is a parsed LINE#HASH reference.
type Anchor struct {
	Line int // 1-based
	Hash string
}

// ParseSpec parses "12#KT" or "12#KT-18#MQ" into start/end anchors.
// For the single form, end == start.
func ParseSpec(spec string) (start, end Anchor, ok bool) {
	m := anchorRe.FindStringSubmatch(spec)
	if m == nil {
		return Anchor{}, Anchor{}, false
	}
	sl, err := strconv.Atoi(m[1])
	if err != nil || sl <= 0 {
		return Anchor{}, Anchor{}, false
	}
	start = Anchor{Line: sl, Hash: m[2]}
	if m[3] == "" {
		return start, start, true
	}
	el, err := strconv.Atoi(m[3])
	if err != nil || el <= 0 {
		return Anchor{}, Anchor{}, false
	}
	end = Anchor{Line: el, Hash: m[4]}
	return start, end, true
}

// Verify reports whether anchor a matches the content of fileLines.
func Verify(fileLines []string, a Anchor) bool {
	idx := a.Line - 1
	if idx < 0 || idx >= len(fileLines) {
		return false
	}
	return ComputeLineHash(fileLines, idx) == a.Hash
}

// ResolveSpec parses an anchor spec and verifies both endpoints against the
// file content. On success it returns the resolved 1-based [start, end] range.
// Strict by design: a hash mismatch returns ok=false — no fuzzy relocation.
func ResolveSpec(spec string, fileContent string) (startLine, endLine int, ok bool) {
	if spec == "" || fileContent == "" {
		return 0, 0, false
	}
	start, end, ok := ParseSpec(spec)
	if !ok {
		return 0, 0, false
	}
	if end.Line < start.Line {
		start, end = end, start
	}
	lines := strings.Split(fileContent, "\n")
	if !Verify(lines, start) || !Verify(lines, end) {
		return 0, 0, false
	}
	return start.Line, end.Line, true
}
