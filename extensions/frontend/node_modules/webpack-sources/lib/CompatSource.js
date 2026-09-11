/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const Source = require("./Source");

/** @typedef {import("./Source").ClearCacheOptions} ClearCacheOptions */
/** @typedef {import("./Source").HashLike} HashLike */
/** @typedef {import("./Source").MapOptions} MapOptions */
/** @typedef {import("./Source").RawSourceMap} RawSourceMap */
/** @typedef {import("./Source").SourceAndMap} SourceAndMap */
/** @typedef {import("./Source").SourceValue} SourceValue */

/**
 * @typedef {object} SourceLike
 * @property {() => SourceValue} source source
 * @property {(() => Buffer)=} buffer buffer
 * @property {(() => Buffer[])=} buffers buffers
 * @property {(() => number)=} size size
 * @property {((options?: MapOptions) => RawSourceMap | null)=} map map
 * @property {((options?: MapOptions) => SourceAndMap)=} sourceAndMap source and map
 * @property {((hash: HashLike) => void)=} updateHash hash updater
 * @property {((options?: ClearCacheOptions, visited?: WeakSet<Source>) => void)=} clearCache clear cache
 */

class CompatSource extends Source {
	/**
	 * @param {SourceLike} sourceLike source like
	 * @returns {Source} source
	 */
	static from(sourceLike) {
		return sourceLike instanceof Source
			? sourceLike
			: new CompatSource(sourceLike);
	}

	/**
	 * @param {SourceLike} sourceLike source like
	 */
	constructor(sourceLike) {
		super();
		/**
		 * @private
		 * @type {SourceLike}
		 */
		this._sourceLike = sourceLike;
	}

	/**
	 * @returns {SourceValue} source
	 */
	source() {
		return this._sourceLike.source();
	}

	/**
	 * @returns {Buffer} buffer
	 */
	buffer() {
		if (typeof this._sourceLike.buffer === "function") {
			return this._sourceLike.buffer();
		}
		return super.buffer();
	}

	/**
	 * @returns {Buffer[]} buffers
	 */
	buffers() {
		if (typeof this._sourceLike.buffers === "function") {
			return this._sourceLike.buffers();
		}
		return super.buffers();
	}

	/**
	 * @returns {number} size
	 */
	size() {
		if (typeof this._sourceLike.size === "function") {
			return this._sourceLike.size();
		}
		return super.size();
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {RawSourceMap | null} map
	 */
	map(options) {
		if (typeof this._sourceLike.map === "function") {
			return this._sourceLike.map(options);
		}
		return super.map(options);
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {SourceAndMap} source and map
	 */
	sourceAndMap(options) {
		if (typeof this._sourceLike.sourceAndMap === "function") {
			return this._sourceLike.sourceAndMap(options);
		}
		return super.sourceAndMap(options);
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
		if (visited !== undefined) visited.add(this);
		const sourceLike = this._sourceLike;
		if (typeof sourceLike.clearCache === "function") {
			sourceLike.clearCache(options, visited);
		}
	}

	/**
	 * @param {HashLike} hash hash
	 * @returns {void}
	 */
	updateHash(hash) {
		if (typeof this._sourceLike.updateHash === "function") {
			return this._sourceLike.updateHash(hash);
		}
		if (typeof this._sourceLike.map === "function") {
			throw new Error(
				"A Source-like object with a 'map' method must also provide an 'updateHash' method",
			);
		}
		hash.update(this.buffer());
	}
}

module.exports = CompatSource;
