/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const Source = require("./Source");
const { getMap, getSourceAndMap } = require("./helpers/getFromStreamChunks");
const getGeneratedSourceInfo = require("./helpers/getGeneratedSourceInfo");
const { eachPotentialToken } = require("./helpers/splitIntoPotentialTokens");
const {
	isDualStringBufferCachingEnabled,
} = require("./helpers/stringBufferUtils");

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

class OriginalSource extends Source {
	/**
	 * @param {string | Buffer} value value
	 * @param {string} name name
	 */
	constructor(value, name) {
		super();

		const isBuffer = Buffer.isBuffer(value);

		/**
		 * @private
		 * @type {undefined | string}
		 */
		this._value = isBuffer ? undefined : value;
		/**
		 * @private
		 * @type {undefined | Buffer}
		 */
		this._valueAsBuffer = isBuffer ? value : undefined;
		/**
		 * @private
		 * @type {string}
		 */
		this._name = name;
	}

	getName() {
		return this._name;
	}

	/**
	 * @returns {SourceValue} source
	 */
	source() {
		if (this._value === undefined) {
			const value =
				/** @type {Buffer} */
				(this._valueAsBuffer).toString("utf8");
			if (isDualStringBufferCachingEnabled()) {
				this._value = value;
			}
			return value;
		}
		return this._value;
	}

	/**
	 * @returns {Buffer} buffer
	 */
	buffer() {
		if (this._valueAsBuffer === undefined) {
			const value = Buffer.from(/** @type {string} */ (this._value), "utf8");
			if (isDualStringBufferCachingEnabled()) {
				this._valueAsBuffer = value;
			}
			return value;
		}
		return this._valueAsBuffer;
	}

	/**
	 * @returns {number} size
	 */
	size() {
		if (this._cachedSize !== undefined) return this._cachedSize;
		if (this._valueAsBuffer !== undefined) {
			return (this._cachedSize = this._valueAsBuffer.length);
		}
		return (this._cachedSize = Buffer.byteLength(
			/** @type {string} */ (this._value),
			"utf8",
		));
	}

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
	 * @param {OnName} _onName called for each name
	 * @returns {GeneratedSourceInfo} generated source info
	 */
	streamChunks(options, onChunk, onSource, _onName) {
		if (this._value === undefined) {
			this._value =
				/** @type {Buffer} */
				(this._valueAsBuffer).toString("utf8");
		}
		onSource(0, this._name, this._value);
		const finalSource = Boolean(options && options.finalSource);
		if (!options || options.columns !== false) {
			// With column info we need to walk every potential token. The
			// scan is streamed by offset (see `eachPotentialToken`) so we
			// only allocate a chunk substring when one is actually emitted —
			// and `map()` / `sourceAndMap()` set `finalSource`, in which case
			// the chunk is dropped and no substring is allocated at all.
			const value = this._value;
			let line = 1;
			let column = 0;
			eachPotentialToken(value, (start, end, newline) => {
				const length = end - start;
				if (newline && length === 1) {
					if (!finalSource) onChunk("\n", line, column, -1, -1, -1, -1);
				} else {
					const chunk = finalSource ? undefined : value.slice(start, end);
					onChunk(chunk, line, column, 0, line, column, -1);
				}
				if (newline) {
					line++;
					column = 0;
				} else {
					column += length;
				}
			});
			return {
				generatedLine: line,
				generatedColumn: column,
				source: finalSource ? value : undefined,
			};
		} else if (finalSource) {
			// Without column info and with final source we only
			// need meta info to generate mapping
			const result = getGeneratedSourceInfo(this._value);
			const { generatedLine, generatedColumn } = result;
			if (generatedColumn === 0) {
				for (
					let line = 1;
					line < /** @type {number} */ (generatedLine);
					line++
				) {
					onChunk(undefined, line, 0, 0, line, 0, -1);
				}
			} else {
				for (
					let line = 1;
					line <= /** @type {number} */ (generatedLine);
					line++
				) {
					onChunk(undefined, line, 0, 0, line, 0, -1);
				}
			}
			return result;
		}
		// Without column info, but also without final source.
		// We only get here when (options.columns === false && !finalSource),
		// so the source field is always undefined and the chunk arg is always
		// the line text. Single-pass scan over newlines avoids the
		// splitIntoLines array allocation.
		const value = this._value;
		const len = value.length;
		if (len === 0) {
			return { generatedLine: 1, generatedColumn: 0, source: undefined };
		}
		let line = 1;
		let i = 0;
		while (i < len) {
			const n = value.indexOf("\n", i);
			if (n === -1) {
				const lastLine = i === 0 ? value : value.slice(i);
				onChunk(lastLine, line, 0, 0, line, 0, -1);
				return {
					generatedLine: line,
					generatedColumn: lastLine.length,
					source: undefined,
				};
			}
			const chunk = n === i ? "\n" : value.slice(i, n + 1);
			onChunk(chunk, line, 0, 0, line, 0, -1);
			line++;
			i = n + 1;
		}
		// Source ended with a newline.
		return { generatedLine: line, generatedColumn: 0, source: undefined };
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
		if (visited !== undefined) {
			if (visited.has(this)) return;
			visited.add(this);
		}
		if (options !== undefined && options.source === false) return;
		if (this._value !== undefined && this._valueAsBuffer !== undefined) {
			// When both forms are cached, drop the string (UTF-16 in V8 is
			// ~2 bytes/char) and keep the more compact buffer. source() will
			// rehydrate the string on demand.
			this._value = undefined;
		}
	}

	/**
	 * @param {HashLike} hash hash
	 * @returns {void}
	 */
	updateHash(hash) {
		hash.update("OriginalSource");
		hash.update(this.buffer());
		hash.update(this._name || "");
	}
}

module.exports = OriginalSource;
