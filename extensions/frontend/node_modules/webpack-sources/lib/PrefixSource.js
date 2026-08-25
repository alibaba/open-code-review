/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const RawSource = require("./RawSource");
const Source = require("./Source");
const { getMap, getSourceAndMap } = require("./helpers/getFromStreamChunks");
const streamChunks = require("./helpers/streamChunks");

/** @typedef {import("./Source").ClearCacheOptions} ClearCacheOptions */
/** @typedef {import("./Source").HashLike} HashLike */
/** @typedef {import("./Source").MapOptions} MapOptions */
/** @typedef {import("./Source").RawSourceMap} RawSourceMap */
/** @typedef {import("./Source").SourceAndMap} SourceAndMap */
/** @typedef {import("./Source").SourceValue} SourceValue */
/** @typedef {import("./helpers/getGeneratedSourceInfo").GeneratedSourceInfo} GeneratedSourceInfo */
/** @typedef {import("./helpers/streamChunks").OnChunk} OnChunk */
/** @typedef {import("./helpers/streamChunks").OnName} OnName */
/** @typedef {import("./helpers/streamChunks").OnSource} OnSource */
/** @typedef {import("./helpers/streamChunks").Options} Options */

// `/\n/g` (no lookahead) lets V8's regex compiler take its
// literal-character fast path; the previous `/\n(?=.|\s)/g` form
// disabled it. Output stays identical because `buildPrefixed` strips
// the spurious trailing prefix when the input ended with a newline.
const NEWLINE_REGEX = /\n/g;

/**
 * Prepend `prefix` and insert `prefix` after every newline that has
 * content following — the original `/\n(?=.|\s)/g` semantics, but
 * implemented as a fast-path regex + tail-strip.
 * @param {string} prefix prefix
 * @param {string} node underlying source string
 * @returns {string} prefixed string
 */
const buildPrefixed = (prefix, node) => {
	if (prefix.length === 0) return node;
	const replaced = node.replace(NEWLINE_REGEX, `\n${prefix}`);
	const len = node.length;
	// `/\n/g` matches the trailing newline too, so the replace appended
	// a spurious prefix at the end. Trim it.
	if (len > 0 && node.charCodeAt(len - 1) === 10) {
		return prefix + replaced.slice(0, replaced.length - prefix.length);
	}
	return prefix + replaced;
};

class PrefixSource extends Source {
	/**
	 * @param {string} prefix prefix
	 * @param {string | Buffer | Source} source source
	 */
	constructor(prefix, source) {
		super();
		/**
		 * @private
		 * @type {string}
		 */
		this._prefix = prefix;
		/**
		 * @private
		 * @type {Source}
		 */
		this._source =
			typeof source === "string" || Buffer.isBuffer(source)
				? new RawSource(source, true)
				: source;
	}

	getPrefix() {
		return this._prefix;
	}

	original() {
		return this._source;
	}

	/**
	 * @returns {SourceValue} source
	 */
	source() {
		return buildPrefixed(
			this._prefix,
			/** @type {string} */ (this._source.source()),
		);
	}

	// buffer() / buffers() / size() inherit from Source.prototype.
	// Source.buffer() does Buffer.from(this.source(), "utf8") — cheaper
	// in CodSpeed instruction count than any safe override we tried
	// (caching is unsafe with mutable child; a JS-side splice loop
	// regressed ~5x in instruction count). Speeding up source() via the
	// simpler regex above lifts buffer(), buffers(), size(), and any
	// other `this.source()`-using path with no override needed.

	/**
	 * @param {MapOptions=} options map options
	 * @returns {RawSourceMap | null} map
	 */
	map(options) {
		return getMap(this, options);
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {SourceAndMap} source and map
	 */
	sourceAndMap(options) {
		return getSourceAndMap(this, options);
	}

	/**
	 * @param {Options} options options
	 * @param {OnChunk} onChunk called for each chunk of code
	 * @param {OnSource} onSource called for each source
	 * @param {OnName} onName called for each name
	 * @returns {GeneratedSourceInfo} generated source info
	 */
	streamChunks(options, onChunk, onSource, onName) {
		const prefix = this._prefix;
		const prefixOffset = prefix.length;
		const linesOnly = Boolean(options && options.columns === false);
		const { generatedLine, generatedColumn, source } = streamChunks(
			this._source,
			options,
			(
				chunk,
				generatedLine,
				generatedColumn,
				sourceIndex,
				originalLine,
				originalColumn,
				nameIndex,
			) => {
				if (generatedColumn !== 0) {
					// In the middle of the line, we just adject the column
					generatedColumn += prefixOffset;
				} else if (chunk !== undefined) {
					// At the start of the line, when we have source content
					// add the prefix as generated mapping
					// (in lines only mode we just add it to the original mapping
					// for performance reasons)
					if (linesOnly || sourceIndex < 0) {
						chunk = prefix + chunk;
					} else if (prefixOffset > 0) {
						onChunk(prefix, generatedLine, generatedColumn, -1, -1, -1, -1);
						generatedColumn += prefixOffset;
					}
				} else if (!linesOnly) {
					// Without source content, we only need to adject the column info
					// expect in lines only mode where prefix is added to original mapping
					generatedColumn += prefixOffset;
				}
				onChunk(
					chunk,
					generatedLine,
					generatedColumn,
					sourceIndex,
					originalLine,
					originalColumn,
					nameIndex,
				);
			},
			onSource,
			onName,
		);
		return {
			generatedLine,
			generatedColumn:
				generatedColumn === 0
					? 0
					: prefixOffset + /** @type {number} */ (generatedColumn),
			source: source !== undefined ? buildPrefixed(prefix, source) : undefined,
		};
	}

	/**
	 * @param {HashLike} hash hash
	 * @returns {void}
	 */
	updateHash(hash) {
		hash.update("PrefixSource");
		this._source.updateHash(hash);
		hash.update(this._prefix);
	}

	/**
	 * Release cached data held by this source. clearCache is a memory
	 * hint: it never affects correctness or output, only how expensive
	 * the next read is. Subclasses override; the base is a no-op so
	 * every Source supports the call. Composite sources always recurse
	 * into wrapped sources. When the same child is reachable via several
	 * parents (e.g. modules shared across webpack chunks), pass a shared
	 * `visited` WeakSet so each subtree is walked at most once.
	 * Not safe to call concurrently with source/map/sourceAndMap/
	 * streamChunks/updateHash on the same instance.
	 * @param {ClearCacheOptions=} options selectors
	 * @param {WeakSet<Source>=} visited de-duplication set shared across calls
	 * @returns {void}
	 */
	clearCache(options, visited) {
		if (visited !== undefined && visited.has(this)) return;
		let v = visited;
		if (v === undefined) v = new WeakSet();
		v.add(this);
		this._source.clearCache(options, v);
	}
}

module.exports = PrefixSource;
