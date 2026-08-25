/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const Source = require("./Source");
const { getMap, getSourceAndMap } = require("./helpers/getFromStreamChunks");
const splitIntoLines = require("./helpers/splitIntoLines");
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

// since v8 7.0, Array.prototype.sort is stable
const hasStableSort =
	typeof process === "object" &&
	process.versions &&
	typeof process.versions.v8 === "string" &&
	!/^[0-6]\./.test(process.versions.v8);

// This is larger than max string length
const MAX_SOURCE_POSITION = 0x20000000;

/**
 * Stable comparator hoisted to module scope so each `_sortReplacements()`
 * call doesn't allocate a fresh closure.
 * @param {Replacement} a a
 * @param {Replacement} b b
 * @returns {number} order
 */
const compareStable = (a, b) => {
	const diff1 = a.start - b.start;
	if (diff1 !== 0) return diff1;
	const diff2 = a.end - b.end;
	if (diff2 !== 0) return diff2;
	return 0;
};

/**
 * Index-stabilising comparator for v8 < 7.0 (pre-stable Array.prototype.sort).
 * Unreachable on any supported Node — the `hasStableSort` guard always
 * wins so coverage tools never see this execute.
 * @param {Replacement} a a
 * @param {Replacement} b b
 * @returns {number} order
 */
/* istanbul ignore next */
const compareUnstableFallback = (a, b) => {
	const diff1 = a.start - b.start;
	if (diff1 !== 0) return diff1;
	const diff2 = a.end - b.end;
	if (diff2 !== 0) return diff2;
	return /** @type {number} */ (a.index) - /** @type {number} */ (b.index);
};

class Replacement {
	/**
	 * @param {number} start start
	 * @param {number} end end
	 * @param {string} content content
	 * @param {string=} name name
	 */
	constructor(start, end, content, name) {
		this.start = start;
		this.end = end;
		this.content = content;
		this.name = name;
		// V8 < 7.0 only — unreachable on any supported Node.
		/* istanbul ignore if */
		if (!hasStableSort) {
			this.index = -1;
		}
	}
}

class ReplaceSource extends Source {
	/**
	 * @param {Source} source source
	 * @param {string=} name name
	 */
	constructor(source, name) {
		super();
		/**
		 * @private
		 * @type {Source}
		 */
		this._source = source;
		/**
		 * @private
		 * @type {string | undefined}
		 */
		this._name = name;
		/** @type {Replacement[]} */
		this._replacements = [];
		/**
		 * @private
		 * @type {boolean}
		 */
		this._isSorted = true;
	}

	getName() {
		return this._name;
	}

	getReplacements() {
		this._sortReplacements();
		return this._replacements;
	}

	/**
	 * @param {number} start start
	 * @param {number} end end
	 * @param {string} newValue new value
	 * @param {string=} name name
	 * @returns {void}
	 */
	replace(start, end, newValue, name) {
		if (typeof newValue !== "string") {
			throw new Error(
				`insertion must be a string, but is a ${typeof newValue}`,
			);
		}
		this._replacements.push(new Replacement(start, end, newValue, name));
		this._isSorted = false;
	}

	/**
	 * @param {number} pos pos
	 * @param {string} newValue new value
	 * @param {string=} name name
	 * @returns {void}
	 */
	insert(pos, newValue, name) {
		if (typeof newValue !== "string") {
			throw new Error(
				`insertion must be a string, but is a ${typeof newValue}: ${newValue}`,
			);
		}
		this._replacements.push(new Replacement(pos, pos - 1, newValue, name));
		this._isSorted = false;
	}

	/**
	 * @returns {SourceValue} source
	 */
	source() {
		if (this._replacements.length === 0) {
			return this._source.source();
		}
		const current = /** @type {string} */ (this._source.source());
		let pos = 0;
		const result = [];

		this._sortReplacements();
		for (const replacement of this._replacements) {
			const start = Math.floor(replacement.start);
			const end = Math.floor(replacement.end + 1);
			if (pos < start) {
				// slice directly from the original string rather than repeatedly
				// producing smaller intermediate strings, which avoids O(n) copies.
				result.push(current.slice(pos, start));
				pos = start;
			}
			result.push(replacement.content);
			if (pos < end) {
				pos = end;
			}
		}
		if (pos < current.length) {
			result.push(pos === 0 ? current : current.slice(pos));
		}
		return result.join("");
	}

	/**
	 * @returns {Buffer} buffer
	 */
	buffer() {
		if (this._replacements.length === 0) {
			return this._source.buffer();
		}
		return super.buffer();
	}

	/**
	 * @returns {Buffer[]} buffers
	 */
	buffers() {
		if (this._replacements.length === 0) {
			// TODO remove in the next major release
			return typeof this._source.buffers === "function"
				? this._source.buffers()
				: [this._source.buffer()];
		}
		return [this.buffer()];
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {RawSourceMap | null} map
	 */
	map(options) {
		if (this._replacements.length === 0) {
			return this._source.map(options);
		}
		return getMap(this, options);
	}

	/**
	 * @param {MapOptions=} options map options
	 * @returns {SourceAndMap} source and map
	 */
	sourceAndMap(options) {
		if (this._replacements.length === 0) {
			return this._source.sourceAndMap(options);
		}
		return getSourceAndMap(this, options);
	}

	original() {
		return this._source;
	}

	_sortReplacements() {
		if (this._isSorted) return;
		const replacements = this._replacements;
		// Replacements are usually appended in source order (ties keep
		// insertion order, matching a stable sort), so an O(n) pre-scan
		// often lets us skip the sort and its per-element comparator calls.
		let isPresorted = true;
		for (let i = 1; i < replacements.length; i++) {
			const prev = replacements[i - 1];
			const repl = replacements[i];
			if (
				repl.start < prev.start ||
				(repl.start === prev.start && repl.end < prev.end)
			) {
				isPresorted = false;
				break;
			}
		}
		if (isPresorted) {
			this._isSorted = true;
			return;
		}
		if (hasStableSort) {
			this._replacements.sort(compareStable);
		} else {
			// V8 < 7.0 only — unreachable on any supported Node.
			/* istanbul ignore next */
			for (const [i, repl] of this._replacements.entries()) repl.index = i;
			/* istanbul ignore next */
			this._replacements.sort(compareUnstableFallback);
		}
		this._isSorted = true;
	}

	/**
	 * @param {Options} options options
	 * @param {OnChunk} onChunk called for each chunk of code
	 * @param {OnSource} onSource called for each source
	 * @param {OnName} onName called for each name
	 * @returns {GeneratedSourceInfo} generated source info
	 */
	streamChunks(options, onChunk, onSource, onName) {
		this._sortReplacements();
		// When the consumer only wants the final source (map() /
		// sourceAndMap()), emit position-only chunks (chunk === undefined,
		// like OriginalSource and RawSource do) and hand back the whole
		// replaced source once at the end. This avoids allocating boundary
		// slices for emission and — more importantly — the per-chunk
		// `code += chunk` cons-string chain in every enclosing consumer.
		// With `source: false` the caller (getMap) additionally promises not
		// to read the returned source, so its assembly is skipped entirely;
		// streamAndGetSourceAndMap overrides that flag because it caches the
		// text.
		const finalSource = Boolean(options && options.finalSource);
		const needSource = !options || options.source !== false;
		const replacements = this._replacements;
		let pos = 0;
		let i = 0;
		let replacementEnd = -1;
		let nextReplacement =
			i < replacements.length
				? Math.floor(replacements[i].start)
				: MAX_SOURCE_POSITION;
		let generatedLineOffset = 0;
		let generatedColumnOffset = 0;
		let generatedColumnOffsetLine = 0;
		/** @type {(string | undefined)[]} */
		const sourceContents = [];
		/**
		 * Lazily-built line-start offsets per source content. One number per
		 * line instead of one substring per line (`splitIntoLines`), and the
		 * chunk comparison below runs allocation-free via `startsWith`.
		 * @type {(number[] | undefined)[]}
		 */
		const sourceContentLineStarts = [];
		/** @type {Map<string, number>} */
		const nameMapping = new Map();
		/** @type {number[]} */
		const nameIndexMapping = [];
		/**
		 * @param {number} sourceIndex source index
		 * @param {number} line line
		 * @param {number} column column
		 * @param {string} expectedChunk expected chunk
		 * @returns {boolean} result
		 */
		const checkOriginalContent = (sourceIndex, line, column, expectedChunk) => {
			const content =
				sourceIndex < sourceContents.length
					? sourceContents[sourceIndex]
					: undefined;
			if (content === undefined) return false;
			let lineStarts = sourceContentLineStarts[sourceIndex];
			if (lineStarts === undefined) {
				// Line boundaries mirror `splitIntoLines`: every line includes
				// its trailing "\n"; a final line without "\n" still counts.
				lineStarts = [];
				const { length } = content;
				let offset = 0;
				while (offset < length) {
					lineStarts.push(offset);
					const newline = content.indexOf("\n", offset);
					if (newline === -1) break;
					offset = newline + 1;
				}
				sourceContentLineStarts[sourceIndex] = lineStarts;
			}
			if (line > lineStarts.length) return false;
			const lineStart = lineStarts[line - 1];
			const lineEnd =
				line < lineStarts.length ? lineStarts[line] : content.length;
			// The expected chunk never spans lines, so a match must fit into
			// the current line (mirrors the old per-line slice comparison).
			if (column + expectedChunk.length > lineEnd - lineStart) return false;
			return content.startsWith(expectedChunk, lineStart + column);
		};
		const { generatedLine, generatedColumn } = streamChunks(
			this._source,
			{ ...options, finalSource: false },
			(
				_chunk,
				generatedLine,
				generatedColumn,
				sourceIndex,
				originalLine,
				originalColumn,
				nameIndex,
			) => {
				let chunkPos = 0;
				const chunk = /** @type {string} */ (_chunk);
				const endPos = pos + chunk.length;

				// Skip over when it has been replaced
				if (replacementEnd > pos) {
					// Skip over the whole chunk
					if (replacementEnd >= endPos) {
						const line = generatedLine + generatedLineOffset;
						if (chunk.endsWith("\n")) {
							generatedLineOffset--;
							if (generatedColumnOffsetLine === line) {
								// undo exiting corrections form the current line
								generatedColumnOffset += generatedColumn;
							}
						} else if (generatedColumnOffsetLine === line) {
							generatedColumnOffset -= chunk.length;
						} else {
							/* istanbul ignore next: pre-existing chunk-skipping cross-line case (also untested on main) */
							generatedColumnOffset = -chunk.length;
							/* istanbul ignore next: pre-existing chunk-skipping cross-line case (also untested on main) */
							generatedColumnOffsetLine = line;
						}
						pos = endPos;
						return;
					}

					// Partially skip over chunk
					chunkPos = replacementEnd - pos;
					if (
						checkOriginalContent(
							sourceIndex,
							originalLine,
							originalColumn,
							chunk.slice(0, chunkPos),
						)
					) {
						originalColumn += chunkPos;
					}
					pos += chunkPos;
					const line = generatedLine + generatedLineOffset;
					/* istanbul ignore else: pre-existing chunk-skipping cross-line case (also untested on main) */
					if (generatedColumnOffsetLine === line) {
						generatedColumnOffset -= chunkPos;
					} else {
						generatedColumnOffset = -chunkPos;
						generatedColumnOffsetLine = line;
					}
					generatedColumn += chunkPos;
				}

				// Is a replacement in the chunk?
				if (nextReplacement < endPos) {
					do {
						let line = generatedLine + generatedLineOffset;
						if (nextReplacement > pos) {
							// Emit chunk until replacement
							const offset = nextReplacement - pos;
							const chunkSlice = chunk.slice(chunkPos, chunkPos + offset);
							onChunk(
								finalSource ? undefined : chunkSlice,
								line,
								generatedColumn +
									(line === generatedColumnOffsetLine
										? generatedColumnOffset
										: 0),
								sourceIndex,
								originalLine,
								originalColumn,
								nameIndex < 0 || nameIndex >= nameIndexMapping.length
									? -1
									: nameIndexMapping[nameIndex],
							);
							generatedColumn += offset;
							chunkPos += offset;
							pos = nextReplacement;
							if (
								checkOriginalContent(
									sourceIndex,
									originalLine,
									originalColumn,
									chunkSlice,
								)
							) {
								originalColumn += chunkSlice.length;
							}
						}

						// Insert replacement content splitted into chunks by lines
						const { content, name } = replacements[i];
						let replacementNameIndex = nameIndex;
						if (sourceIndex >= 0 && name) {
							let globalIndex = nameMapping.get(name);
							if (globalIndex === undefined) {
								globalIndex = nameMapping.size;
								nameMapping.set(name, globalIndex);
								onName(globalIndex, name);
							}
							replacementNameIndex = globalIndex;
						}
						// Fast path: most replacements (renamed identifiers,
						// short inserts) carry single-line content. Skip
						// `splitIntoLines` — and its array allocation — when
						// we can tell the content has no embedded newline.
						// `splitIntoLines("")` returns `[]`; emitting a zero-
						// length chunk would still walk the loop, so handle
						// it as a no-op explicitly.
						if (content.length > 0 && !content.includes("\n")) {
							onChunk(
								finalSource ? undefined : content,
								line,
								generatedColumn +
									(line === generatedColumnOffsetLine
										? generatedColumnOffset
										: 0),
								sourceIndex,
								originalLine,
								originalColumn,
								replacementNameIndex,
							);
							if (generatedColumnOffsetLine === line) {
								generatedColumnOffset += content.length;
							} else {
								generatedColumnOffset = content.length;
								generatedColumnOffsetLine = line;
							}
						} else if (content.length === 0) {
							// Empty replacement: no chunk to emit, no column
							// movement. `splitIntoLines("")` is `[]` so the
							// existing loop already does nothing — explicit
							// guard skips the per-call array allocation.
						} else {
							const matches = splitIntoLines(content);
							for (let m = 0; m < matches.length; m++) {
								const contentLine = matches[m];
								onChunk(
									finalSource ? undefined : contentLine,
									line,
									generatedColumn +
										(line === generatedColumnOffsetLine
											? generatedColumnOffset
											: 0),
									sourceIndex,
									originalLine,
									originalColumn,
									replacementNameIndex,
								);

								// Only the first chunk has name assigned
								replacementNameIndex = -1;

								if (m === matches.length - 1 && !contentLine.endsWith("\n")) {
									/* istanbul ignore else: pre-existing multi-line replacement cross-line case (also untested on main) */
									if (generatedColumnOffsetLine === line) {
										generatedColumnOffset += contentLine.length;
									} else {
										generatedColumnOffset = contentLine.length;
										generatedColumnOffsetLine = line;
									}
								} else {
									generatedLineOffset++;
									line++;
									generatedColumnOffset = -generatedColumn;
									generatedColumnOffsetLine = line;
								}
							}
						}

						// Remove replaced content by settings this variable
						replacementEnd = Math.max(
							replacementEnd,
							Math.floor(replacements[i].end + 1),
						);

						// Move to next replacement
						i++;
						nextReplacement =
							i < replacements.length
								? Math.floor(replacements[i].start)
								: MAX_SOURCE_POSITION;

						// Skip over when it has been replaced
						const offset = chunk.length - endPos + replacementEnd - chunkPos;
						if (offset > 0) {
							// Skip over whole chunk
							if (replacementEnd >= endPos) {
								const line = generatedLine + generatedLineOffset;
								if (chunk.endsWith("\n")) {
									generatedLineOffset--;
									if (generatedColumnOffsetLine === line) {
										// undo exiting corrections form the current line
										generatedColumnOffset += generatedColumn;
									}
								} else if (generatedColumnOffsetLine === line) {
									generatedColumnOffset -= chunk.length - chunkPos;
								} else {
									generatedColumnOffset = chunkPos - chunk.length;
									generatedColumnOffsetLine = line;
								}
								pos = endPos;
								return;
							}

							// Partially skip over chunk
							const line = generatedLine + generatedLineOffset;
							if (
								checkOriginalContent(
									sourceIndex,
									originalLine,
									originalColumn,
									chunk.slice(chunkPos, chunkPos + offset),
								)
							) {
								originalColumn += offset;
							}
							chunkPos += offset;
							pos += offset;
							if (generatedColumnOffsetLine === line) {
								generatedColumnOffset -= offset;
							} else {
								generatedColumnOffset = -offset;
								generatedColumnOffsetLine = line;
							}
							generatedColumn += offset;
						}
					} while (nextReplacement < endPos);
				}

				// Emit remaining chunk
				if (chunkPos < chunk.length) {
					const chunkSlice = finalSource
						? undefined
						: chunkPos === 0
							? chunk
							: chunk.slice(chunkPos);
					const line = generatedLine + generatedLineOffset;
					onChunk(
						chunkSlice,
						line,
						generatedColumn +
							(line === generatedColumnOffsetLine ? generatedColumnOffset : 0),
						sourceIndex,
						originalLine,
						originalColumn,
						nameIndex < 0 ? -1 : nameIndexMapping[nameIndex],
					);
				}
				pos = endPos;
			},
			(sourceIndex, source, sourceContent) => {
				/* istanbul ignore next: non-sequential sourceIndex emission is not produced by any in-tree Source */
				while (sourceContents.length < sourceIndex) {
					sourceContents.push(undefined);
				}
				sourceContents[sourceIndex] = sourceContent;
				onSource(sourceIndex, source, sourceContent);
			},
			(nameIndex, name) => {
				let globalIndex = nameMapping.get(name);
				if (globalIndex === undefined) {
					globalIndex = nameMapping.size;
					nameMapping.set(name, globalIndex);
					onName(globalIndex, name);
				}
				nameIndexMapping[nameIndex] = globalIndex;
			},
		);

		// Handle remaining replacements
		let remainer = "";
		for (; i < replacements.length; i++) {
			remainer += replacements[i].content;
		}

		// Insert remaining replacements content splitted into chunks by lines
		let line = /** @type {number} */ (generatedLine) + generatedLineOffset;
		// Fast path mirroring the in-chunk replacement loop above: skip
		// splitIntoLines + per-match loop when the trailing content has no
		// newlines (the common case when remaining replacements are single
		// inserts).
		if (remainer.length > 0 && !remainer.includes("\n")) {
			onChunk(
				finalSource ? undefined : remainer,
				line,
				/** @type {number} */
				(generatedColumn) +
					(line === generatedColumnOffsetLine ? generatedColumnOffset : 0),
				-1,
				-1,
				-1,
				-1,
			);
			/* istanbul ignore else: trailing-remainer cross-line case (also untested on main) */
			if (generatedColumnOffsetLine === line) {
				generatedColumnOffset += remainer.length;
			} else {
				generatedColumnOffset = remainer.length;
				generatedColumnOffsetLine = line;
			}
		} else {
			const matches = splitIntoLines(remainer);
			for (let m = 0; m < matches.length; m++) {
				const contentLine = matches[m];
				onChunk(
					finalSource ? undefined : contentLine,
					line,
					/** @type {number} */
					(generatedColumn) +
						(line === generatedColumnOffsetLine ? generatedColumnOffset : 0),
					-1,
					-1,
					-1,
					-1,
				);

				if (m === matches.length - 1 && !contentLine.endsWith("\n")) {
					/* istanbul ignore else: trailing-remainer multi-line cross-line case (also untested on main) */
					if (generatedColumnOffsetLine === line) {
						generatedColumnOffset += contentLine.length;
					} else {
						generatedColumnOffset = contentLine.length;
						generatedColumnOffsetLine = line;
					}
				} else {
					generatedLineOffset++;
					line++;
					generatedColumnOffset = -(/** @type {number} */ (generatedColumn));
					generatedColumnOffsetLine = line;
				}
			}
		}

		return {
			generatedLine: line,
			generatedColumn:
				/** @type {number} */
				(generatedColumn) +
				(line === generatedColumnOffsetLine ? generatedColumnOffset : 0),
			// The streamed chunks reproduce source() exactly, so the final
			// source can be assembled in O(replacements) string operations
			// instead of re-concatenating every emitted chunk downstream.
			source:
				finalSource && needSource
					? /** @type {string} */ (this.source())
					: undefined,
		};
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

	/**
	 * @param {HashLike} hash hash
	 * @returns {void}
	 */
	updateHash(hash) {
		this._sortReplacements();
		hash.update("ReplaceSource");
		this._source.updateHash(hash);
		hash.update(this._name || "");
		// Feed each replacement as multiple updates instead of building one
		// combined template literal per replacement. The resulting digest is
		// identical (hash.update is additive over bytes), but we avoid
		// allocating a new string per replacement.
		for (const repl of this._replacements) {
			hash.update(`${repl.start}${repl.end}`);
			hash.update(repl.content);
			if (repl.name) hash.update(repl.name);
		}
	}
}

module.exports = ReplaceSource;
module.exports.Replacement = Replacement;
