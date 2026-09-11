/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const { createMappingsWriter } = require("./createMappingsSerializer");

/** @typedef {import("../Source").RawSourceMap} RawSourceMap */
/** @typedef {import("../Source").SourceAndMap} SourceAndMap */
/** @typedef {import("./streamChunks").Options} Options */
/** @typedef {import("./streamChunks").StreamChunksFunction} StreamChunksFunction */

/** @typedef {{ streamChunks: StreamChunksFunction }} SourceLikeWithStreamChunks */

/**
 * Assign `value` at index `i` of `arr`, padding earlier slots with `null`.
 * Hoisted helper so `getMap` and `getSourceAndMap` share one
 * implementation. Uses `push` rather than `arr.length = i + 1` because the
 * latter forces V8's HOLEY_ELEMENTS kind permanently — push keeps the
 * backing store PACKED, which the downstream source-map serializer iterates
 * over far more efficiently.
 * @template T
 * @param {(T | null)[]} arr destination array
 * @param {number} i index to assign
 * @param {T} value value to assign
 * @returns {void}
 */
const setAtIndex = (arr, i, value) => {
	while (arr.length < i) arr.push(null);
	// `arr[i] = value` where `i === arr.length` extends the array exactly
	// like `push` (V8 keeps PACKED_ELEMENTS); after the while-loop
	// `i <= arr.length` always, so the hole case that would force HOLEY is
	// impossible here.
	arr[i] = value;
};

/**
 * @param {SourceLikeWithStreamChunks} source source
 * @param {Options=} options options
 * @returns {RawSourceMap | null} map
 */
module.exports.getMap = (source, options) => {
	/** @type {(string | null)[]} */
	const potentialSources = [];
	/** @type {(string | null)[]} */
	const potentialSourcesContent = [];
	/** @type {(string | null)[]} */
	const potentialNames = [];
	const mappingsWriter = createMappingsWriter(options);
	const addMapping = mappingsWriter.add;
	source.streamChunks(
		{ ...options, source: false, finalSource: true },
		(
			chunk,
			generatedLine,
			generatedColumn,
			sourceIndex,
			originalLine,
			originalColumn,
			nameIndex,
		) => {
			addMapping(
				generatedLine,
				generatedColumn,
				sourceIndex,
				originalLine,
				originalColumn,
				nameIndex,
			);
		},
		(sourceIndex, source, sourceContent) => {
			setAtIndex(potentialSources, sourceIndex, source);
			if (sourceContent !== undefined) {
				setAtIndex(potentialSourcesContent, sourceIndex, sourceContent);
			}
		},
		(nameIndex, name) => {
			setAtIndex(potentialNames, nameIndex, name);
		},
	);
	const mappings = mappingsWriter.finish();
	return mappings.length > 0
		? {
				version: 3,
				file: "x",
				mappings,
				// We handle broken sources as `null`, in spec this field should be string, but no information what we should do in such cases if we change type it will be breaking change
				sources: /** @type {string[]} */ (potentialSources),
				sourcesContent:
					potentialSourcesContent.length > 0
						? /** @type {string[]} */ (potentialSourcesContent)
						: undefined,
				names: /** @type {string[]} */ (potentialNames),
			}
		: null;
};

/**
 * @param {SourceLikeWithStreamChunks} inputSource input source
 * @param {Options=} options options
 * @returns {SourceAndMap} map
 */
module.exports.getSourceAndMap = (inputSource, options) => {
	let code = "";
	/** @type {(string | null)[]} */
	const potentialSources = [];
	/** @type {(string | null)[]} */
	const potentialSourcesContent = [];
	/** @type {(string | null)[]} */
	const potentialNames = [];
	const mappingsWriter = createMappingsWriter(options);
	const addMapping = mappingsWriter.add;
	const { source } = inputSource.streamChunks(
		// `source: true` overrides any `source: false` a caller forwarded
		// (e.g. the streamChunks fallback passing getMap options through
		// sourceAndMap) — this helper always consumes the source.
		{ ...options, finalSource: true, source: true },
		(
			chunk,
			generatedLine,
			generatedColumn,
			sourceIndex,
			originalLine,
			originalColumn,
			nameIndex,
		) => {
			if (chunk !== undefined) code += chunk;
			addMapping(
				generatedLine,
				generatedColumn,
				sourceIndex,
				originalLine,
				originalColumn,
				nameIndex,
			);
		},
		(sourceIndex, source, sourceContent) => {
			while (potentialSources.length < sourceIndex) {
				potentialSources.push(null);
			}
			potentialSources[sourceIndex] = source;
			if (sourceContent !== undefined) {
				while (potentialSourcesContent.length < sourceIndex) {
					potentialSourcesContent.push(null);
				}
				potentialSourcesContent[sourceIndex] = sourceContent;
			}
		},
		(nameIndex, name) => {
			while (potentialNames.length < nameIndex) {
				potentialNames.push(null);
			}
			potentialNames[nameIndex] = name;
		},
	);
	const mappings = mappingsWriter.finish();
	return {
		source: source !== undefined ? source : code,
		map:
			mappings.length > 0
				? {
						version: 3,
						file: "x",
						mappings,
						// We handle broken sources as `null`, in spec this field should be string, but no information what we should do in such cases if we change type it will be breaking change
						sources: /** @type {string[]} */ (potentialSources),
						sourcesContent:
							potentialSourcesContent.length > 0
								? /** @type {string[]} */ (potentialSourcesContent)
								: undefined,
						names: /** @type {string[]} */ (potentialNames),
					}
				: null,
	};
};
