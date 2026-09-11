/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

// Character classification via a lookup table.  A single bitmask test
// replaces the multi-comparison chains in each inner loop phase.
//
// BIT layout per character:
//   bit 0 (STOP1 = 1): stops phase-1 scan  (\n ; { })
//   bit 1 (CONT2 = 2): continues phase-2 scan  (; { } space \r \t)
//
// Phase 1: scan regular source chars that are NOT a phase-1 stop.
// Phase 2: consume runs of statement-boundary / whitespace chars.
// Phase 3: consume a trailing \n if present.

const STOP1 = 1;
const CONT2 = 2;

/** @type {Uint8Array} */
const CF = new Uint8Array(128);
CF[10] = STOP1; // \n  – stops phase 1, NOT consumed in phase 2
CF[59] = STOP1 | CONT2; // ;
CF[123] = STOP1 | CONT2; // {
CF[125] = STOP1 | CONT2; // }
CF[32] = CONT2; // space
CF[13] = CONT2; // \r
CF[9] = CONT2; // \t

/**
 * @callback OnPotentialToken
 * @param {number} start start offset (inclusive)
 * @param {number} end end offset (exclusive)
 * @param {boolean} newline whether the token ends with a `\n`
 * @returns {void}
 */

/**
 * Streaming core: report each potential token by its `[start, end)` bounds
 * instead of materialising substrings. The single real consumer
 * (`OriginalSource.streamChunks`) slices on demand — and skips slicing
 * entirely when emitting the final source (the `map()` / `sourceAndMap()`
 * paths, which discard the chunk text) — so this avoids both the
 * intermediate results array and every per-token `String.slice` allocation
 * in the dominant case.
 * @param {string} str string
 * @param {OnPotentialToken} onToken called for each token
 * @returns {void}
 */
const eachPotentialToken = (str, onToken) => {
	const len = str.length;
	let i = 0;
	outer: while (i < len) {
		const start = i;
		// Phase 1 – skip regular (non-stop) characters
		let cc = str.charCodeAt(i);
		while (cc > 127 || !(CF[cc] & STOP1)) {
			if (++i >= len) {
				onToken(start, i, false);
				break outer;
			}
			cc = str.charCodeAt(i);
		}
		// Phase 2 – consume delimiter / whitespace run (; { } space \r \t)
		while (cc < 128 && CF[cc] & CONT2) {
			if (++i >= len) {
				onToken(start, i, false);
				break outer;
			}
			cc = str.charCodeAt(i);
		}
		// Phase 3 – consume trailing newline
		if (cc === 10) {
			i++;
			onToken(start, i, true);
		} else {
			onToken(start, i, false);
		}
	}
};

/**
 * Array-returning variant. Kept as a standalone loop rather than wrapping
 * `eachPotentialToken` with a per-token callback: the callback indirection
 * measurably slows this hot scan (V8 can no longer inline the slice/push),
 * and the two only share the same small, well-tested classification table.
 * @param {string} str string
 * @returns {string[] | null} array of string separated by potential tokens
 */
const splitIntoPotentialTokens = (str) => {
	const len = str.length;
	if (len === 0) return null;
	const results = [];
	let i = 0;
	outer: while (i < len) {
		const start = i;
		// Phase 1 – skip regular (non-stop) characters
		let cc = str.charCodeAt(i);
		while (cc > 127 || !(CF[cc] & STOP1)) {
			if (++i >= len) {
				results.push(str.slice(start, i));
				break outer;
			}
			cc = str.charCodeAt(i);
		}
		// Phase 2 – consume delimiter / whitespace run (; { } space \r \t)
		while (cc < 128 && CF[cc] & CONT2) {
			if (++i >= len) {
				results.push(str.slice(start, i));
				break outer;
			}
			cc = str.charCodeAt(i);
		}
		// Phase 3 – consume trailing newline
		if (cc === 10) {
			i++;
		}
		results.push(str.slice(start, i));
	}
	return results;
};

module.exports = splitIntoPotentialTokens;
module.exports.eachPotentialToken = eachPotentialToken;
