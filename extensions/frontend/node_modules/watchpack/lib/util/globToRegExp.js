/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Haijie Xie @hai-x
*/

"use strict";

// Based on https://github.com/fitzgen/glob-to-regexp (MIT)
// Specialized for watchpack: `extended` and `globstar` always enabled.
// Returns the regexp source without `^`/`$` anchors.

const CC_EXCLAMATION = 33; // "!"
const CC_DOLLAR = 36; // "$"
const CC_LEFT_PARENTHESIS = 40; // "("
const CC_RIGHT_PARENTHESIS = 41; // ")"
const CC_ASTERISK = 42; // "*"
const CC_PLUS = 43; // "+"
const CC_COMMA = 44; // ","
const CC_DOT = 46; // "."
const CC_SLASH = 47; // "/"
const CC_EQUAL = 61; // "="
const CC_QUESTION_MARK = 63; // "?"
const CC_LEFT_BRACKET = 91; // "["
const CC_RIGHT_BRACKET = 93; // "]"
const CC_CARET = 94; // "^"
const CC_LEFT_BRACE = 123; // "{"
const CC_PIPE = 124; // "|"
const CC_RIGHT_BRACE = 125; // "}"

/**
 * @param {string} glob glob pattern
 * @returns {string} regexp source without anchors
 */
module.exports = (glob) => {
	if (typeof glob !== "string") {
		throw new TypeError("Expected a string");
	}

	const len = glob.length;
	let reStr = "";
	let inGroup = false;
	// Start of the current run of literal characters, copied with one slice
	let literalStart = 0;

	for (let i = 0; i < len; i++) {
		const cc = glob.charCodeAt(i);
		const tokenStart = i;
		let mapped;

		switch (cc) {
			case CC_SLASH:
				mapped = "\\/";
				break;
			case CC_DOLLAR:
				mapped = "\\$";
				break;
			case CC_CARET:
				mapped = "\\^";
				break;
			case CC_PLUS:
				mapped = "\\+";
				break;
			case CC_DOT:
				mapped = "\\.";
				break;
			case CC_LEFT_PARENTHESIS:
				mapped = "\\(";
				break;
			case CC_RIGHT_PARENTHESIS:
				mapped = "\\)";
				break;
			case CC_EQUAL:
				mapped = "\\=";
				break;
			case CC_EXCLAMATION:
				mapped = "\\!";
				break;
			case CC_PIPE:
				mapped = "\\|";
				break;
			case CC_QUESTION_MARK:
				mapped = ".";
				break;
			case CC_LEFT_BRACKET:
				mapped = "[";
				break;
			case CC_RIGHT_BRACKET:
				mapped = "]";
				break;
			case CC_LEFT_BRACE:
				inGroup = true;
				mapped = "(";
				break;
			case CC_RIGHT_BRACE:
				inGroup = false;
				mapped = ")";
				break;
			case CC_COMMA:
				mapped = inGroup ? "|" : "\\,";
				break;
			case CC_ASTERISK: {
				const atStart = i === 0;
				const afterSlash = !atStart && glob.charCodeAt(i - 1) === CC_SLASH;
				let starCount = 1;
				while (i + 1 < len && glob.charCodeAt(i + 1) === CC_ASTERISK) {
					starCount++;
					i++;
				}
				const atEnd = i + 1 === len;
				const beforeSlash = !atEnd && glob.charCodeAt(i + 1) === CC_SLASH;
				if (
					starCount > 1 &&
					(atStart || afterSlash) &&
					(atEnd || beforeSlash)
				) {
					// Globstar segment, matches zero or more path segments
					mapped = "((?:[^/]*(?:\\/|$))*)";
					i++; // Move over the "/"
				} else {
					// Not a globstar, matches one path segment
					mapped = "([^/]*)";
				}
				break;
			}
			default:
				// Literal character, extend the current run
				continue;
		}

		if (literalStart < tokenStart) {
			reStr += glob.slice(literalStart, tokenStart);
		}
		reStr += mapped;
		literalStart = i + 1;
	}

	if (literalStart < len) {
		reStr += literalStart === 0 ? glob : glob.slice(literalStart);
	}

	return reStr;
};
