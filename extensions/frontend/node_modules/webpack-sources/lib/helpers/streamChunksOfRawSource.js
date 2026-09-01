/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const getGeneratedSourceInfo = require("./getGeneratedSourceInfo");

/** @typedef {import("./getGeneratedSourceInfo").GeneratedSourceInfo} GeneratedSourceInfo */
/** @typedef {import("./streamChunks").OnChunk} OnChunk */
/** @typedef {import("./streamChunks").OnName} OnName */
/** @typedef {import("./streamChunks").OnSource} OnSource */

/**
 * @param {string} source source
 * @param {OnChunk} onChunk on chunk
 * @param {OnSource} _onSource on source
 * @param {OnName} _onName on name
 * @returns {GeneratedSourceInfo} source info
 *
 * Single-pass equivalent of `splitIntoLines(source).forEach(emit)` — emits
 * each line via onChunk while scanning for newlines, so we never allocate
 * the intermediate array of lines.
 */
const streamChunksOfRawSource = (source, onChunk, _onSource, _onName) => {
	const len = source.length;
	if (len === 0) {
		return { generatedLine: 1, generatedColumn: 0 };
	}
	let line = 1;
	let i = 0;
	while (i < len) {
		const n = source.indexOf("\n", i);
		if (n === -1) {
			// Trailing partial line (no \n). Emit and return its length as the
			// final column.
			const lastLine = i === 0 ? source : source.slice(i);
			onChunk(lastLine, line, 0, -1, -1, -1, -1);
			return { generatedLine: line, generatedColumn: lastLine.length };
		}
		const chunk = n === i ? "\n" : source.slice(i, n + 1);
		onChunk(chunk, line, 0, -1, -1, -1, -1);
		line++;
		i = n + 1;
	}
	// Source ended with a newline — the next "logical" line is empty.
	return { generatedLine: line, generatedColumn: 0 };
};

/**
 * @param {string} source source
 * @param {OnChunk} onChunk on chunk
 * @param {OnSource} onSource on source
 * @param {OnName} onName on name
 * @param {boolean} finalSource is final source
 * @returns {GeneratedSourceInfo} source info
 */
module.exports = (source, onChunk, onSource, onName, finalSource) =>
	finalSource
		? getGeneratedSourceInfo(source)
		: streamChunksOfRawSource(source, onChunk, onSource, onName);
