/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

/**
 * @typedef {object} GeneratedSourceInfo
 * @property {number=} generatedLine generated line
 * @property {number=} generatedColumn generated column
 * @property {string=} source source
 */

/**
 * @param {string | undefined} source source
 * @returns {GeneratedSourceInfo} source info
 */
const getGeneratedSourceInfo = (source) => {
	if (source === undefined) {
		return {};
	}
	const lastLineStart = source.lastIndexOf("\n");
	if (lastLineStart === -1) {
		return {
			generatedLine: 1,
			generatedColumn: source.length,
			source,
		};
	}
	// Use native indexOf to scan for newlines instead of charCodeAt loops.
	// This is significantly faster on large sources since indexOf uses
	// vectorized/native string scanning.
	let generatedLine = 2;
	let idx = source.indexOf("\n");
	while (idx !== -1 && idx < lastLineStart) {
		generatedLine++;
		idx = source.indexOf("\n", idx + 1);
	}
	return {
		generatedLine,
		generatedColumn: source.length - lastLineStart - 1,
		source,
	};
};

module.exports = getGeneratedSourceInfo;
