/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const Source = require("./Source");
const streamChunksOfRawSource = require("./helpers/streamChunksOfRawSource");
const {
	internString,
	isDualStringBufferCachingEnabled,
} = require("./helpers/stringBufferUtils");

/** @typedef {import("./Source").ClearCacheOptions} ClearCacheOptions */
/** @typedef {import("./Source").HashLike} HashLike */
/** @typedef {import("./Source").MapOptions} MapOptions */
/** @typedef {import("./Source").RawSourceMap} RawSourceMap */
/** @typedef {import("./Source").SourceValue} SourceValue */
/** @typedef {import("./helpers/getGeneratedSourceInfo").GeneratedSourceInfo} GeneratedSourceInfo */
/** @typedef {import("./helpers/streamChunks").OnChunk} OnChunk */
/** @typedef {import("./helpers/streamChunks").OnName} OnName */
/** @typedef {import("./helpers/streamChunks").OnSource} OnSource */
/** @typedef {import("./helpers/streamChunks").Options} Options */

class RawSource extends Source {
	/**
	 * @param {string | Buffer} value value
	 * @param {boolean=} convertToString convert to string
	 */
	constructor(value, convertToString = false) {
		super();
		const isBuffer = Buffer.isBuffer(value);
		if (isBuffer) {
			/**
			 * @private
			 * @type {boolean}
			 */
			this._valueIsBuffer = !convertToString;
			/**
			 * @private
			 * @type {undefined | string | Buffer}
			 */
			this._value = convertToString ? undefined : value;
			/**
			 * @private
			 * @type {undefined | Buffer}
			 */
			this._valueAsBuffer = value;
			/**
			 * @private
			 * @type {undefined | string}
			 */
			this._valueAsString = undefined;
		} else if (typeof value === "string") {
			const interned = internString(value);
			this._valueIsBuffer = false;
			this._value = interned;
			this._valueAsBuffer = undefined;
			this._valueAsString = interned;
		} else {
			throw new TypeError("argument 'value' must be either string or Buffer");
		}
	}

	isBuffer() {
		return this._valueIsBuffer;
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
				this._value = internString(value);
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
			/** @type {string} */ (this._valueAsString),
			"utf8",
		));
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {RawSourceMap | null} map
	 */
	// eslint-disable-next-line no-unused-vars
	map(options) {
		return null;
	}

	/**
	 * @param {Options} options options
	 * @param {OnChunk} onChunk called for each chunk of code
	 * @param {OnSource} onSource called for each source
	 * @param {OnName} onName called for each name
	 * @returns {GeneratedSourceInfo} generated source info
	 */
	streamChunks(options, onChunk, onSource, onName) {
		let strValue = this._valueAsString;
		if (strValue === undefined) {
			const value = this.source();
			strValue = typeof value === "string" ? value : value.toString("utf8");
			if (isDualStringBufferCachingEnabled()) {
				this._valueAsString = internString(strValue);
			}
		}
		return streamChunksOfRawSource(
			strValue,
			onChunk,
			onSource,
			onName,
			Boolean(options && options.finalSource),
		);
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
		if (this._valueIsBuffer) {
			// Buffer is the primary representation (and lives in `_value`);
			// only the string form, populated lazily by streamChunks, is
			// safe to drop.
			this._valueAsString = undefined;
		} else if (this._valueAsBuffer !== undefined && this._value !== undefined) {
			// The source was constructed from a string (so `_value` and
			// `_valueAsString` reference the same interned string) and the
			// buffer form was materialized later by `buffer()` /
			// `updateHash()`. The buffer is therefore safe to drop.
			this._valueAsBuffer = undefined;
		}
	}

	/**
	 * @param {HashLike} hash hash
	 * @returns {void}
	 */
	updateHash(hash) {
		hash.update("RawSource");
		hash.update(this.buffer());
	}
}

module.exports = RawSource;
