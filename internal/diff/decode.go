// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"strings"
	"unicode/utf8"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/textenc"
)

// hunkFrame remembers the shape of a diff section so decoded payload bodies can
// be written back exactly where they came from, with their original prefix
// bytes. Every line that is not hunk payload — the "diff --git" header, "---",
// "+++", "index", "@@" headers, rename/mode lines, "Binary files ... differ",
// "\ No newline at end of file", and everything outside a hunk — is carried
// through byte for byte and never decoded. Filenames in particular stay raw.
type hunkFrame struct {
	lines  []string // the whole section, split on "\n"
	idx    []int    // indices into lines that hold payload
	prefix []byte   // the single prefix byte stripped from each payload line
}

// splitHunkPayload separates a diff section into the reviewable payload (the
// added, deleted and context line bodies, joined with "\n") and the frame that
// can put it back.
//
// Classification comes from the parser's inHunk state plus the first byte,
// never from matching header text. That is not a stylistic choice: git emits a
// deleted SQL line "-- old comment" as "--- old comment" INSIDE a hunk, and an
// added line "++i" as "+++i". A prefix-list classifier reads both as headers
// and leaves them mojibake while decoding their neighbours.
func splitHunkPayload(section string) (string, *hunkFrame) {
	lines := strings.Split(section, "\n")
	f := &hunkFrame{lines: lines}
	var bodies []string

	inHunk := false
	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		// One prefix byte per line, matching ParseHunks. The legacy charsets
		// in the allowlist never place 0x2B ("+"), 0x2D ("-"), 0x20 (" "),
		// 0x0A or 0x0D inside a multi-byte sequence, so splitting on "\n" and
		// stripping one byte cannot cut a character in half. A line starting
		// with 0x5C inside a hunk can only be the "\ No newline" marker, and
		// falls out as frame here without a text match.
		if c := line[0]; c != '+' && c != '-' && c != ' ' {
			continue
		}
		f.idx = append(f.idx, i)
		f.prefix = append(f.prefix, line[0])
		bodies = append(bodies, line[1:])
	}
	return strings.Join(bodies, "\n"), f
}

// rebuild writes decoded payload bodies back into the frame. ok is false if the
// decode changed the number of lines, in which case the caller must keep the
// section raw: a diff whose line count moved no longer describes the file.
func (f *hunkFrame) rebuild(decoded string) (string, bool) {
	if len(f.idx) == 0 {
		return strings.Join(f.lines, "\n"), true
	}
	bodies := strings.Split(decoded, "\n")
	if len(bodies) != len(f.idx) {
		return "", false
	}
	out := make([]string, len(f.lines))
	copy(out, f.lines)
	for k, i := range f.idx {
		out[i] = string(f.prefix[k]) + bodies[k]
	}
	return strings.Join(out, "\n"), true
}

// decodeDiffFile runs charset detection ONCE per file and applies the answer to
// both payloads the reviewer produces: the hunk text inside d.Diff — which the
// prompt substitutes into {{diff}}, and is therefore what the model actually
// reads — and d.NewFileContent, which the line resolver scans.
//
// A diff carries both sides of the change: "-" lines are OLD bytes, "+" and
// context lines are NEW bytes. A commit can change the file's encoding, and
// then one charset cannot serve both sides. The frame records the side of every
// payload line, and that is what decides how the line is decoded:
//
//   - New-side lines are decoded with the file's charset whenever the file
//     itself is legacy, with no utf8.Valid test. Their bytes ARE the file's
//     bytes, so the file's answer covers them by construction.
//   - Old-side lines live in the commit and not in the file, and the parser
//     only ever has HEAD, so their own bytes are the only evidence there is:
//     they are decoded only when they are not already valid UTF-8.
//
// Deciding the new side by its bytes instead would carry through raw any legacy
// line whose bytes are also valid UTF-8 — for GB-18030 that needs every
// character to be lead 0xC2-0xDF with trail 0x80-0xBF, which 1920 of its CJK
// characters satisfy, and Big5, EUC-JP and EUC-KR overlap the same way. The
// result is well-formed UTF-8 spelling something else: no guard fires, nothing
// is marked, and the text is wrong.
//
// Context lines count as new-side. git compares bytes, so a commit that
// re-encodes a file leaves a hunk with no context lines at all — every line
// differs. A context line therefore carries the same bytes on both sides.
//
// The file stays the preferred detection evidence wherever it carries a signal.
// git runs with -U3, so a hunk payload carrying one short CJK comment scores
// below the confidence gate while the whole file scores 100 — detecting from
// the payload when the file could answer would leave that hunk mojibake. The
// bad payload lines are the evidence only when the file is valid UTF-8 and
// therefore says nothing about the old side.
//
// fileRaw is nil when the new-file bytes are unavailable (deleted file,
// unreadable path); the hunk payload is then the only evidence. That is also
// the case for an untracked file, whose synthesized diff has the whole file as
// its payload.
//
// Four known ceilings, none of them closable at this seam. When only one or
// two short "-" lines are legacy their union can score below the confidence
// gate, so the file is marked rather than decoded — guessing a charset from ten
// bytes produces wrong-but-plausible CJK that nothing downstream can detect.
//
// A legacy-to-legacy conversion, say Big5 to GB-18030, decodes the old "-"
// lines with the new side's charset, and because those charsets accept each
// other's bytes the result is plausible CJK with no U+FFFD for Convert's gain
// check to see. Only the base blob would fix it.
//
// A DELETED line whose legacy bytes are also valid UTF-8 is carried through
// raw: the side rule closes this for the new side, but without a base ref the
// line's own bytes are all a "-" line has to go on.
//
// UTF-16 is excluded rather than decoded, deliberately: git marks such files
// binary so they normally return at the IsBinary check below, and a repo that
// forces a text diff (`*.java diff` in .gitattributes) yields a hunk that git
// split on 0x0A and therefore mid-code-unit, leaving every payload line
// byte-shifted and BOM-less, so the per-line Convert fails and the file is
// marked undecoded and dropped. Converting the joined payload in one shot is
// not the fix: git inserts the +/-/space prefix byte at those same shifted
// boundaries, so one-shot decoding would produce plausible mojibake with no
// U+FFFD for the gain check to catch.
//
// Everything commits together or nothing does: the model can never see d.Diff
// decoded beside a raw d.NewFileContent, or one payload line decoded beside a
// raw neighbour.
func decodeDiffFile(d *model.Diff, fileRaw []byte) {
	if d.IsBinary {
		return
	}

	payload, frame := splitHunkPayload(d.Diff)
	// Line-by-line decoding is equivalent to decoding the whole payload for
	// the allowlisted charsets: none of them places 0x0A inside a multi-byte
	// sequence, and their decoders carry no state across lines.
	bodies := strings.Split(payload, "\n")
	fileIsUTF8 := len(fileRaw) > 0 && utf8.Valid(fileRaw)
	// The new-file bytes are legacy, so every new-side payload line is legacy
	// too, however its own bytes read.
	newSideIsLegacy := len(fileRaw) > 0 && !fileIsUTF8

	// frame.prefix is aligned index-for-index with the bodies, except when the
	// section carries no payload at all: strings.Split("", "\n") is [""] while
	// frame.idx is empty. With no side to read, a line falls back to its bytes.
	sides := frame.prefix
	if len(bodies) != len(sides) {
		sides = nil
	}
	var bad []int
	for i, body := range bodies {
		newSide := sides != nil && sides[i] != '-'
		if (newSideIsLegacy && newSide) || !utf8.ValidString(body) {
			bad = append(bad, i)
		}
	}

	// Nothing to decode: every payload line is already UTF-8, and so is the
	// file (or there is no file). Byte-identical fast path.
	if len(bad) == 0 && (len(fileRaw) == 0 || fileIsUTF8) {
		return
	}

	evidence := fileRaw
	switch {
	case fileIsUTF8:
		lines := make([]string, len(bad))
		for k, i := range bad {
			lines[k] = bodies[i]
		}
		evidence = []byte(strings.Join(lines, "\n"))
	case len(fileRaw) == 0:
		evidence = []byte(payload)
	}

	// Detect cannot answer UTF-8 here. Every path that reaches this point has
	// non-empty evidence that is invalid UTF-8: the joined bad lines are
	// invalid by definition, and so is a file that failed utf8.Valid. The
	// valid-UTF-8 case already returned above.
	charset, confidence, ok := textenc.Detect(evidence)
	if !ok {
		markUndecoded(d, charset, evidence, fileIsUTF8)
		return
	}

	fileText, okFile := "", true
	if len(fileRaw) > 0 && !fileIsUTF8 {
		fileText, okFile = textenc.Convert(charset, fileRaw)
	}
	okLines := true
	for _, i := range bad {
		text, okLine := textenc.Convert(charset, []byte(bodies[i]))
		if !okLine {
			okLines = false
			break
		}
		bodies[i] = text
	}
	rebuilt, okFrame := frame.rebuild(strings.Join(bodies, "\n"))
	if !okFile || !okLines || !okFrame {
		markUndecoded(d, charset, evidence, fileIsUTF8)
		return
	}

	if len(fileRaw) > 0 && !fileIsUTF8 {
		d.NewFileContent = fileText
	}
	d.Diff = rebuilt
	textenc.Info(decodePath(d), charset, confidence)
}

// markUndecoded records the detector's answer on the model and decides, from
// the raw bytes, whether the file is merely imperfect (keep reviewing it, which
// is what it gets today) or past reviewing (exclude it). Marking is
// unconditional; exclusion is not.
//
// newSideIsUTF8 says the new-file bytes are already valid UTF-8, so only some
// deleted lines are legacy: the content the resolver scans and nearly all of
// the payload the model reads are real text. Excluding such a file would review
// less than we do today, so it is marked and kept.
func markUndecoded(d *model.Diff, charset string, evidence []byte, newSideIsUTF8 bool) {
	d.UndecodedCharset = charset
	d.Unreviewable = !newSideIsUTF8 && textenc.Unreviewable(charset, evidence)
}

// decodePath names the file for the info line, preferring the new path.
func decodePath(d *model.Diff) string {
	if d.NewPath == "" || d.NewPath == "/dev/null" {
		return d.OldPath
	}
	return d.NewPath
}
