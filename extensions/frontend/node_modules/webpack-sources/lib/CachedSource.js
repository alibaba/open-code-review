/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const Source = require("./Source");
const streamAndGetSourceAndMap = require("./helpers/streamAndGetSourceAndMap");
const streamChunksOfRawSource = require("./helpers/streamChunksOfRawSource");
const streamChunksOfSourceMap = require("./helpers/streamChunksOfSourceMap");
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

/**
 * @typedef {object} BufferedMap
 * @property {number} version version
 * @property {string[]} sources sources
 * @property {string[]} names name
 * @property {string=} sourceRoot source root
 * @property {(Buffer | "")[]=} sourcesContent sources content
 * @property {Buffer=} mappings mappings
 * @property {string} file file
 */

/**
 * @param {null | RawSourceMap} map map
 * @returns {null | BufferedMap} buffered map
 */
const mapToBufferedMap = (map) => {
	if (typeof map !== "object" || !map) return map;
	const bufferedMap =
		/** @type {BufferedMap} */
		(/** @type {unknown} */ ({ ...map }));
	if (map.mappings) {
		bufferedMap.mappings = Buffer.from(map.mappings, "utf8");
	}
	if (map.sourcesContent) {
		bufferedMap.sourcesContent = map.sourcesContent.map(
			(str) => str && Buffer.from(str, "utf8"),
		);
	}
	return bufferedMap;
};

/**
 * @param {null | BufferedMap} bufferedMap buffered map
 * @returns {null | RawSourceMap} map
 */
const bufferedMapToMap = (bufferedMap) => {
	if (typeof bufferedMap !== "object" || !bufferedMap) return bufferedMap;
	const map =
		/** @type {RawSourceMap} */
		(/** @type {unknown} */ ({ ...bufferedMap }));
	if (bufferedMap.mappings) {
		map.mappings = bufferedMap.mappings.toString("utf8");
	}
	if (bufferedMap.sourcesContent) {
		map.sourcesContent = bufferedMap.sourcesContent.map(
			(buffer) => buffer && buffer.toString("utf8"),
		);
	}
	return map;
};

/** @typedef {{ map?: null | RawSourceMap, bufferedMap?: null | BufferedMap }} BufferEntry */
/** @typedef {Map<string, BufferEntry>} BufferedMaps */

const CACHE_KEY_EMPTY = "{}";
const CACHE_KEY_COLUMNS_FALSE = '{"columns":false}';
const CACHE_KEY_COLUMNS_TRUE = '{"columns":true}';

/**
 * Fast-path replacement for `JSON.stringify(options)` when used as a cache
 * key. MapOptions / streamChunks Options are both small boolean-only shapes
 * and the overwhelmingly common shapes (`undefined`, `{}`, `{columns}`) can
 * be keyed without calling `JSON.stringify`, which dominates short-circuit
 * cache lookups. Falls back to `JSON.stringify` for any other shape so keys
 * remain compatible with previously cached `BufferedMaps` entries.
 * @param {undefined | MapOptions | Options} options options
 * @returns {string} cache key
 */
const getCacheKey = (options) => {
	if (!options) return CACHE_KEY_EMPTY;
	const { columns } = options;
	if (
		/** @type {Options} */ (options).source === undefined &&
		/** @type {Options} */ (options).finalSource === undefined &&
		/** @type {MapOptions} */ (options).module === undefined
	) {
		if (columns === undefined) return CACHE_KEY_EMPTY;
		return columns ? CACHE_KEY_COLUMNS_TRUE : CACHE_KEY_COLUMNS_FALSE;
	}
	return JSON.stringify(options);
};

/**
 * @typedef {object} CachedData
 * @property {boolean=} source source
 * @property {Buffer} buffer buffer
 * @property {number=} size size
 * @property {BufferedMaps} maps maps
 * @property {(string | Buffer)[]=} hash hash
 */

class CachedSource extends Source {
	/**
	 * @param {Source | (() => Source)} source source
	 * @param {CachedData=} cachedData cached data
	 */
	constructor(source, cachedData) {
		super();
		/**
		 * @private
		 * @type {Source | (() => Source)}
		 */
		this._source = source;
		/**
		 * @private
		 * @type {undefined | string}
		 */
		this._cachedSource = undefined;
		// Split on `cachedData` once instead of re-evaluating the ternary for
		// every field. Under the interpreter (and CodSpeed's simulation) each
		// ternary is a separate branch; consolidating cuts the per-instance
		// branch count roughly in half.
		if (cachedData) {
			/**
			 * @private
			 * @type {boolean | undefined}
			 */
			this._cachedSourceType = cachedData.source;
			/**
			 * @private
			 * @type {Buffer | undefined}
			 */
			this._cachedBuffer = cachedData.buffer;
			/**
			 * @private
			 * @type {number | undefined}
			 */
			this._cachedSize = cachedData.size;
			/**
			 * @private
			 * @type {BufferedMaps}
			 */
			this._cachedMaps = cachedData.maps;
			/**
			 * @private
			 * @type {(string | Buffer)[] | undefined}
			 */
			this._cachedHashUpdate = cachedData.hash;
		} else {
			this._cachedSourceType = undefined;
			this._cachedBuffer = undefined;
			this._cachedSize = undefined;
			this._cachedMaps = new Map();
			this._cachedHashUpdate = undefined;
		}
	}

	/**
	 * @returns {CachedData} cached data
	 */
	getCachedData() {
		/** @type {BufferedMaps} */
		const bufferedMaps = new Map();
		for (const pair of this._cachedMaps) {
			const [, cacheEntry] = pair;
			if (cacheEntry.bufferedMap === undefined) {
				cacheEntry.bufferedMap = mapToBufferedMap(
					this._getMapFromCacheEntry(cacheEntry),
				);
			}
			bufferedMaps.set(pair[0], {
				map: undefined,
				bufferedMap: cacheEntry.bufferedMap,
			});
		}
		return {
			// `CachedData.buffer` is required (it is the on-disk
			// serialization format consumed by the
			// `new CachedSource(source, cachedData)` constructor).
			// `_cachedBuffer` is populated by `buffer()` calls but a
			// caller may invoke `getCachedData()` after `clearCache()`
			// has dropped it; `this.buffer()` rehydrates via the
			// wrapped source so the contract holds in every state.
			buffer: this.buffer(),
			source:
				this._cachedSourceType !== undefined
					? this._cachedSourceType
					: typeof this._cachedSource === "string"
						? true
						: Buffer.isBuffer(this._cachedSource)
							? false
							: undefined,
			size: this._cachedSize,
			maps: bufferedMaps,
			hash: this._cachedHashUpdate,
		};
	}

	originalLazy() {
		return this._source;
	}

	original() {
		if (typeof this._source === "function") this._source = this._source();
		return this._source;
	}

	/**
	 * @returns {SourceValue} source
	 */
	source() {
		// Fully inlined _getCachedSource: both warm- and cold-cache paths skip
		// the prototype method lookup / stack frame the interpreter would
		// otherwise pay on every call.
		if (this._cachedSource !== undefined) return this._cachedSource;
		const cachedBuffer = this._cachedBuffer;
		const cachedSourceType = this._cachedSourceType;
		if (cachedBuffer !== undefined && cachedSourceType !== undefined) {
			const value = cachedSourceType
				? cachedBuffer.toString("utf8")
				: cachedBuffer;
			if (isDualStringBufferCachingEnabled()) {
				this._cachedSource = /** @type {string} */ (value);
			}
			return /** @type {string} */ (value);
		}
		return (this._cachedSource =
			/** @type {string} */
			(this.original().source()));
	}

	/**
	 * @private
	 * @param {BufferEntry} cacheEntry cache entry
	 * @returns {null | RawSourceMap} raw source map
	 */
	_getMapFromCacheEntry(cacheEntry) {
		if (cacheEntry.map !== undefined) {
			return cacheEntry.map;
		} else if (cacheEntry.bufferedMap !== undefined) {
			return (cacheEntry.map = bufferedMapToMap(cacheEntry.bufferedMap));
		}

		return null;
	}

	/**
	 * @private
	 * @returns {undefined | string} cached source
	 */
	_getCachedSource() {
		if (this._cachedSource !== undefined) return this._cachedSource;
		if (this._cachedBuffer && this._cachedSourceType !== undefined) {
			const value = this._cachedSourceType
				? this._cachedBuffer.toString("utf8")
				: this._cachedBuffer;
			if (isDualStringBufferCachingEnabled()) {
				this._cachedSource = /** @type {string} */ (value);
			}
			return /** @type {string} */ (value);
		}
	}

	/**
	 * @returns {Buffer} buffer
	 */
	buffer() {
		if (this._cachedBuffer !== undefined) return this._cachedBuffer;
		if (this._cachedBuffers !== undefined) {
			return (this._cachedBuffer = Buffer.concat(this._cachedBuffers));
		}
		if (this._cachedSource !== undefined) {
			const value = Buffer.isBuffer(this._cachedSource)
				? this._cachedSource
				: Buffer.from(this._cachedSource, "utf8");
			if (isDualStringBufferCachingEnabled()) {
				this._cachedBuffer = value;
			}
			return value;
		}
		if (typeof this.original().buffer === "function") {
			return (this._cachedBuffer = this.original().buffer());
		}
		const bufferOrString = this.source();
		if (Buffer.isBuffer(bufferOrString)) {
			return (this._cachedBuffer = bufferOrString);
		}
		const value = Buffer.from(bufferOrString, "utf8");
		if (isDualStringBufferCachingEnabled()) {
			this._cachedBuffer = value;
		}
		return value;
	}

	/**
	 * @returns {Buffer[]} buffers
	 */
	buffers() {
		if (this._cachedBuffers !== undefined) return this._cachedBuffers;
		if (this._cachedBuffer !== undefined) {
			return (this._cachedBuffers = [this._cachedBuffer]);
		}
		const original = this.original();
		if (typeof original.buffers === "function") {
			return (this._cachedBuffers = original.buffers());
		}
		return (this._cachedBuffers = [this.buffer()]);
	}

	/**
	 * @returns {number} size
	 */
	size() {
		if (this._cachedSize !== undefined) return this._cachedSize;
		if (this._cachedBuffer !== undefined) {
			return (this._cachedSize = this._cachedBuffer.length);
		}
		const source = this._getCachedSource();
		if (source !== undefined) {
			return (this._cachedSize = Buffer.byteLength(source));
		}
		return (this._cachedSize = this.original().size());
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {SourceAndMap} source and map
	 */
	sourceAndMap(options) {
		const key = getCacheKey(options);
		const cacheEntry = this._cachedMaps.get(key);
		// Look for a cached map
		if (cacheEntry !== undefined) {
			// We have a cached map in some representation
			const map = this._getMapFromCacheEntry(cacheEntry);

			// Either get the cached source or compute it
			return { source: this.source(), map };
		}
		// Look for a cached source
		let source = this._getCachedSource();
		// Compute the map
		let map;
		if (source !== undefined) {
			map = this.original().map(options);
		} else {
			// Compute the source and map together.
			const sourceAndMap = this.original().sourceAndMap(options);
			source = /** @type {string} */ (sourceAndMap.source);
			map = sourceAndMap.map;
			this._cachedSource = source;
		}
		this._cachedMaps.set(key, {
			map,
			bufferedMap: undefined,
		});
		return { source, map };
	}

	/**
	 * @param {Options} options options
	 * @param {OnChunk} onChunk called for each chunk of code
	 * @param {OnSource} onSource called for each source
	 * @param {OnName} onName called for each name
	 * @returns {GeneratedSourceInfo} generated source info
	 */
	streamChunks(options, onChunk, onSource, onName) {
		const key = getCacheKey(options);
		if (
			this._cachedMaps.has(key) &&
			(this._cachedBuffer !== undefined || this._cachedSource !== undefined)
		) {
			const { source, map } = this.sourceAndMap(options);
			if (map) {
				return streamChunksOfSourceMap(
					/** @type {string} */
					(source),
					map,
					onChunk,
					onSource,
					onName,
					Boolean(options && options.finalSource),
					true,
				);
			}
			return streamChunksOfRawSource(
				/** @type {string} */
				(source),
				onChunk,
				onSource,
				onName,
				Boolean(options && options.finalSource),
			);
		}
		const sourceAndMap = streamAndGetSourceAndMap(
			this.original(),
			options,
			onChunk,
			onSource,
			onName,
		);
		this._cachedSource = sourceAndMap.source;
		this._cachedMaps.set(key, {
			map: /** @type {RawSourceMap} */ (sourceAndMap.map),
			bufferedMap: undefined,
		});
		return sourceAndMap.result;
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {RawSourceMap | null} map
	 */
	map(options) {
		const key = getCacheKey(options);
		const cacheEntry = this._cachedMaps.get(key);
		if (cacheEntry !== undefined) {
			return this._getMapFromCacheEntry(cacheEntry);
		}
		const map = this.original().map(options);
		this._cachedMaps.set(key, {
			map,
			bufferedMap: undefined,
		});
		return map;
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
		const clearSource = !options || options.source !== false;
		const clearMaps = !options || options.maps !== false;
		if (clearSource) {
			this._cachedSource = undefined;
			this._cachedSourceType = undefined;
			this._cachedBuffer = undefined;
			this._cachedBuffers = undefined;
		}
		if (clearMaps) {
			// Reusing the Map avoids per-call allocation churn when builds
			// call clearCache thousands of times.
			this._cachedMaps.clear();
		}
		if (typeof this._source !== "function") {
			let v = visited;
			if (v === undefined) v = new WeakSet();
			v.add(this);
			this._source.clearCache(options, v);
		} else if (visited !== undefined) {
			visited.add(this);
		}
	}

	/**
	 * @param {HashLike} hash hash
	 * @returns {void}
	 */
	updateHash(hash) {
		if (this._cachedHashUpdate !== undefined) {
			for (const item of this._cachedHashUpdate) hash.update(item);
			return;
		}
		/** @type {(string | Buffer)[]} */
		const update = [];
		/** @type {string | undefined} */
		let currentString;
		const tracker = {
			/**
			 * @param {string | Buffer} item item
			 * @returns {void}
			 */
			update: (item) => {
				if (typeof item === "string" && item.length < 10240) {
					if (currentString === undefined) {
						currentString = item;
					} else {
						currentString += item;
						if (currentString.length > 102400) {
							update.push(Buffer.from(currentString));
							currentString = undefined;
						}
					}
				} else {
					if (currentString !== undefined) {
						update.push(Buffer.from(currentString));
						currentString = undefined;
					}
					update.push(item);
				}
			},
		};
		this.original().updateHash(/** @type {HashLike} */ (tracker));
		if (currentString !== undefined) {
			update.push(Buffer.from(currentString));
		}
		for (const item of update) hash.update(item);
		this._cachedHashUpdate = update;
	}
}

module.exports = CachedSource;
