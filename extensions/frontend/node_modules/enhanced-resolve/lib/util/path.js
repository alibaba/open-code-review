/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

const path = require("path");
const { fileURLToPath } = require("url");

const CHAR_HASH = "#".charCodeAt(0);
const CHAR_SLASH = "/".charCodeAt(0);
const CHAR_BACKSLASH = "\\".charCodeAt(0);
const CHAR_A = "A".charCodeAt(0);
const CHAR_Z = "Z".charCodeAt(0);
const CHAR_LOWER_A = "a".charCodeAt(0);
const CHAR_LOWER_Z = "z".charCodeAt(0);
const CHAR_DOT = ".".charCodeAt(0);
const CHAR_COLON = ":".charCodeAt(0);

const posixNormalize = path.posix.normalize;
const winNormalize = path.win32.normalize;

/**
 * @enum {number}
 */
const PathType = Object.freeze({
	Empty: 0,
	Normal: 1,
	Relative: 2,
	AbsoluteWin: 3,
	AbsolutePosix: 4,
	Internal: 5,
});

const deprecatedInvalidSegmentRegEx =
	/(^|\\|\/)((\.|%2e)(\.|%2e)?|(n|%6e|%4e)(o|%6f|%4f)(d|%64|%44)(e|%65|%45)(_|%5f)(m|%6d|%4d)(o|%6f|%4f)(d|%64|%44)(u|%75|%55)(l|%6c|%4c)(e|%65|%45)(s|%73|%53))(\\|\/|$)/i;

const invalidSegmentRegEx =
	/(^|\\|\/)((\.|%2e)(\.|%2e)?|(n|%6e|%4e)(o|%6f|%4f)(d|%64|%44)(e|%65|%45)(_|%5f)(m|%6d|%4d)(o|%6f|%4f)(d|%64|%44)(u|%75|%55)(l|%6c|%4c)(e|%65|%45)(s|%73|%53))?(\\|\/|$)/i;

/**
 * @param {string} maybePath a path
 * @returns {PathType} type of path
 */
const getType = (maybePath) => {
	switch (maybePath.length) {
		case 0:
			return PathType.Empty;
		case 1: {
			const c0 = maybePath.charCodeAt(0);
			switch (c0) {
				case CHAR_DOT:
					return PathType.Relative;
				case CHAR_SLASH:
					return PathType.AbsolutePosix;
				case CHAR_HASH:
					return PathType.Internal;
			}
			return PathType.Normal;
		}
		case 2: {
			const c0 = maybePath.charCodeAt(0);
			switch (c0) {
				case CHAR_DOT: {
					const c1 = maybePath.charCodeAt(1);
					switch (c1) {
						case CHAR_DOT:
						case CHAR_SLASH:
							return PathType.Relative;
					}
					return PathType.Normal;
				}
				case CHAR_SLASH:
					return PathType.AbsolutePosix;
				case CHAR_HASH:
					return PathType.Internal;
			}
			const c1 = maybePath.charCodeAt(1);
			if (
				c1 === CHAR_COLON &&
				((c0 >= CHAR_A && c0 <= CHAR_Z) ||
					(c0 >= CHAR_LOWER_A && c0 <= CHAR_LOWER_Z))
			) {
				return PathType.AbsoluteWin;
			}
			if (c0 === CHAR_BACKSLASH && c1 === CHAR_BACKSLASH) {
				return PathType.AbsoluteWin;
			}
			return PathType.Normal;
		}
	}
	const c0 = maybePath.charCodeAt(0);
	switch (c0) {
		case CHAR_DOT: {
			const c1 = maybePath.charCodeAt(1);
			switch (c1) {
				case CHAR_SLASH:
					return PathType.Relative;
				case CHAR_DOT: {
					const c2 = maybePath.charCodeAt(2);
					if (c2 === CHAR_SLASH) return PathType.Relative;
					return PathType.Normal;
				}
			}
			return PathType.Normal;
		}
		case CHAR_SLASH:
			return PathType.AbsolutePosix;
		case CHAR_HASH:
			return PathType.Internal;
	}
	const c1 = maybePath.charCodeAt(1);
	if (c1 === CHAR_COLON) {
		const c2 = maybePath.charCodeAt(2);
		if (
			(c2 === CHAR_BACKSLASH || c2 === CHAR_SLASH) &&
			((c0 >= CHAR_A && c0 <= CHAR_Z) ||
				(c0 >= CHAR_LOWER_A && c0 <= CHAR_LOWER_Z))
		) {
			return PathType.AbsoluteWin;
		}
	}
	// Two leading backslashes root a UNC share (`\\server\share`) or a DOS
	// device path (`\\?\…`, `\\.\…`); `path.win32` reads either as absolute,
	// so both belong on `path.win32` and not on posix.
	if (c0 === CHAR_BACKSLASH && c1 === CHAR_BACKSLASH) {
		return PathType.AbsoluteWin;
	}
	return PathType.Normal;
};

/**
 * @param {string} maybePath a path
 * @returns {string} the normalized path
 */
const normalize = (maybePath) => {
	switch (getType(maybePath)) {
		case PathType.Empty:
			return maybePath;
		case PathType.AbsoluteWin:
			return winNormalize(maybePath);
		case PathType.Relative: {
			const r = posixNormalize(maybePath);
			return getType(r) === PathType.Relative ? r : `./${r}`;
		}
	}
	return posixNormalize(maybePath);
};

/**
 * @param {string} rootPath the root path
 * @param {string | undefined} request the request path
 * @returns {string} the joined path
 */
const join = (rootPath, request) => {
	if (!request) return normalize(rootPath);
	const requestType = getType(request);
	switch (requestType) {
		case PathType.AbsolutePosix:
			return posixNormalize(request);
		case PathType.AbsoluteWin:
			return winNormalize(request);
	}
	switch (getType(rootPath)) {
		case PathType.Normal:
		case PathType.Relative:
		case PathType.AbsolutePosix:
			return posixNormalize(`${rootPath}/${request}`);
		case PathType.AbsoluteWin:
			return winNormalize(`${rootPath}\\${request}`);
	}
	switch (requestType) {
		case PathType.Empty:
			return rootPath;
		case PathType.Relative: {
			const r = posixNormalize(rootPath);
			return getType(r) === PathType.Relative ? r : `./${r}`;
		}
	}
	return posixNormalize(rootPath);
};

/**
 * @param {string} maybePath a path
 * @returns {string} the directory name
 */
const dirname = (maybePath) => {
	switch (getType(maybePath)) {
		case PathType.AbsoluteWin:
			return path.win32.dirname(maybePath);
	}
	return path.posix.dirname(maybePath);
};

/** @typedef {{ fn: (rootPath: string, request: string) => string, cache: Map<string, Map<string, string | undefined>> }} CachedJoin */

/**
 * @returns {CachedJoin} cached join
 */
const createCachedJoin = () => {
	/** @type {CachedJoin["cache"]} */
	const cache = new Map();
	/** @type {CachedJoin["fn"]} */
	const fn = (rootPath, request) => {
		/** @type {string | undefined} */
		let cacheEntry;
		let inner = cache.get(rootPath);
		if (inner === undefined) {
			cache.set(rootPath, (inner = new Map()));
		} else {
			cacheEntry = inner.get(request);
			if (cacheEntry !== undefined) return cacheEntry;
		}
		cacheEntry = join(rootPath, request);
		inner.set(request, cacheEntry);
		return cacheEntry;
	};
	return { fn, cache };
};

/** @typedef {{ fn: (maybePath: string) => string, cache: Map<string, string> }} CachedDirname */

/**
 * @returns {CachedDirname} cached dirname
 */
const createCachedDirname = () => {
	/** @type {CachedDirname["cache"]} */
	const cache = new Map();
	/** @type {CachedDirname["fn"]} */
	const fn = (maybePath) => {
		const cacheEntry = cache.get(maybePath);
		if (cacheEntry !== undefined) return cacheEntry;
		const result = dirname(maybePath);
		cache.set(maybePath, result);
		return result;
	};
	return { fn, cache };
};

/** @typedef {{ fn: (maybePath: string, suffix?: string) => string, cache: Map<string, Map<string | undefined, string | undefined>> }} CachedBasename */

/**
 * @returns {CachedBasename} cached basename
 */
const createCachedBasename = () => {
	/** @type {CachedBasename["cache"]} */
	const cache = new Map();
	/** @type {CachedBasename["fn"]} */
	const fn = (maybePath, suffix) => {
		/** @type {string | undefined} */
		let cacheEntry;
		let inner = cache.get(maybePath);
		if (inner === undefined) {
			cache.set(maybePath, (inner = new Map()));
		} else {
			cacheEntry = inner.get(suffix);
			if (cacheEntry !== undefined) return cacheEntry;
		}
		cacheEntry = path.basename(maybePath, suffix);
		inner.set(suffix, cacheEntry);
		return cacheEntry;
	};
	return { fn, cache };
};

/**
 * Whether `request` is a relative request — i.e. matches `^\.\.?(?:\/|$)`.
 *
 * This is called on every `doResolve` via `UnsafeCachePlugin` and
 * `getInnerRequest`, so the char-code form is meaningfully faster than the
 * equivalent regex test: no regex state machine, no string object churn.
 * @param {string} request request string
 * @returns {boolean} true if request is relative
 */
const isRelativeRequest = (request) => {
	const len = request.length;
	if (len === 0 || request.charCodeAt(0) !== CHAR_DOT) return false;
	if (len === 1) return true; // "."
	const c1 = request.charCodeAt(1);
	if (c1 === CHAR_SLASH) return true; // "./..."
	if (c1 !== CHAR_DOT) return false; // ".x..."
	if (len === 2) return true; // ".."
	return request.charCodeAt(2) === CHAR_SLASH; // "../..."
};

/**
 * Whether this is a Windows path, in which `/` and `\` are interchangeable and
 * paths compare case-insensitively, as opposed to a posix path, in which `\` is
 * an ordinary filename character. Decided by the root — a drive letter or a
 * leading `\` — which is where `path.win32` and `path.posix` disagree about
 * `parse(maybePath).root`, and never by the host platform, since Windows paths
 * are resolved on posix hosts and in browsers too. A path starting with `//`
 * stays posix: `path.win32` reads it as a UNC root, but here it cannot be told
 * apart from a posix path, where `\` has to keep being a filename character.
 * @param {string} maybePath a path
 * @returns {boolean} true, when the path is a Windows path
 */
const isWindowsPath = (maybePath) => {
	const c0 = maybePath.charCodeAt(0);
	if (c0 === CHAR_BACKSLASH) return true;
	if (maybePath.charCodeAt(1) !== CHAR_COLON) return false;
	return (
		(c0 >= CHAR_A && c0 <= CHAR_Z) || (c0 >= CHAR_LOWER_A && c0 <= CHAR_LOWER_Z)
	);
};

/**
 * @param {number} charCode a char code
 * @param {boolean} windowsPath whether `\` separates segments
 * @returns {boolean} true, when the char code separates path segments
 */
const isSeparator = (charCode, windowsPath) =>
	charCode === CHAR_SLASH || (windowsPath && charCode === CHAR_BACKSLASH);

/**
 * @param {string} parentPath parent directory path
 * @param {boolean} windowsPath whether `parentPath` is a Windows path
 * @returns {number} length of `parentPath` without its trailing separators
 */
const parentPathLength = (parentPath, windowsPath) => {
	let end = parentPath.length;
	while (end > 0 && isSeparator(parentPath.charCodeAt(end - 1), windowsPath)) {
		end--;
	}
	return end;
};

/**
 * Cold path of `startsWithPath`: Node lowercases whole paths rather than single
 * characters, which only makes a difference outside of ASCII.
 * @param {string} parentPath parent directory path
 * @param {number} length number of characters to compare
 * @param {string} childPath child path to check
 * @returns {boolean} true, when both prefixes name the same Windows path
 */
const equalsWindowsPrefix = (parentPath, length, childPath) =>
	childPath.slice(0, length).replace(/\//g, "\\").toLowerCase() ===
	parentPath.slice(0, length).replace(/\//g, "\\").toLowerCase();

/**
 * @param {string} parentPath parent directory path
 * @param {number} length number of characters of `parentPath` to compare
 * @param {string} childPath child path to check
 * @param {boolean} windowsPath whether `parentPath` is a Windows path
 * @returns {boolean} true, when `childPath` starts with that prefix
 */
const startsWithPath = (parentPath, length, childPath, windowsPath) => {
	if (childPath.length < length) return false;
	// The common case is an exact prefix of a parent without trailing
	// separators, which `startsWith` answers natively and without a slice. For
	// a posix parent that is the whole answer, only a Windows one has more
	// spellings of the same path to try.
	if (length === parentPath.length) {
		if (childPath.startsWith(parentPath)) return true;
		if (!windowsPath) return false;
	}
	for (let i = 0; i < length; i++) {
		const childCharCode = childPath.charCodeAt(i);
		const parentCharCode = parentPath.charCodeAt(i);
		if (childCharCode === parentCharCode) continue;
		if (!windowsPath) return false;
		// Windows mixes `/` and `\` freely and compares case-insensitively.
		if (isSeparator(childCharCode, true) && isSeparator(parentCharCode, true)) {
			continue;
		}
		if (childCharCode > 127 || parentCharCode > 127) {
			return equalsWindowsPrefix(parentPath, length, childPath);
		}
		const childLower =
			childCharCode >= CHAR_A && childCharCode <= CHAR_Z
				? childCharCode + 32
				: childCharCode;
		const parentLower =
			parentCharCode >= CHAR_A && parentCharCode <= CHAR_Z
				? parentCharCode + 32
				: parentCharCode;
		if (childLower !== parentLower) return false;
	}
	return true;
};

/**
 * Whether childPath is parentPath itself or a path under it, the answer node's
 * `relative(parentPath, childPath)` gives: not escaping upward and not
 * absolute. A trailing separator on the parent is not part of the boundary, so
 * `/a/b/` contains exactly what `/a/b` contains.
 * @param {string} parentPath parent directory path
 * @param {string} childPath child path to check
 * @returns {boolean} true if childPath is parentPath or is under it
 */
const isInside = (parentPath, childPath) => {
	const windowsPath = isWindowsPath(parentPath);
	const length = parentPathLength(parentPath, windowsPath);
	if (!startsWithPath(parentPath, length, childPath, windowsPath)) return false;
	// The parent itself, or a segment boundary right after it so that `/a/b`
	// does not contain the sibling `/a/b-other`.
	return (
		childPath.length === length ||
		isSeparator(childPath.charCodeAt(length), windowsPath)
	);
};

/**
 * Check if childPath is a subdirectory of parentPath. Compares like `isInside`,
 * except that a path is not a subpath of itself.
 *
 * Called from `TsconfigPathsPlugin._selectPathsDataForContext` inside a loop
 * over every tsconfig-paths context on every resolve, so it's worth keeping
 * cheap: a native `startsWith` plus a separator char check answers it, and the
 * character loop only runs for a Windows path that the prefix test missed.
 * @param {string} parentPath parent directory path
 * @param {string} childPath child path to check
 * @returns {boolean} true if childPath is under parentPath
 */
const isSubPath = (parentPath, childPath) => {
	const windowsPath = isWindowsPath(parentPath);
	const length = parentPathLength(parentPath, windowsPath);
	if (childPath.length <= length) return false;
	if (!startsWithPath(parentPath, length, childPath, windowsPath)) return false;
	return isSeparator(childPath.charCodeAt(length), windowsPath);
};

/**
 * Convert a `file:` `URL` instance to a filesystem path; any other input
 * (including plain strings) is returned unchanged. Mirrors Node's `fs`, which
 * treats strings as literal paths and only `URL` objects as URLs (see
 * nodejs/node#17658) — so a directory literally named `file:` is never
 * mistaken for a URL.
 * @param {string | URL} maybeURL a path string or a `file:` `URL` instance
 * @returns {string} a filesystem path
 */
const toPath = (maybeURL) =>
	maybeURL instanceof URL ? fileURLToPath(maybeURL) : maybeURL;

module.exports.PathType = PathType;
module.exports.createCachedBasename = createCachedBasename;
module.exports.createCachedDirname = createCachedDirname;
module.exports.createCachedJoin = createCachedJoin;
module.exports.deprecatedInvalidSegmentRegEx = deprecatedInvalidSegmentRegEx;
module.exports.dirname = dirname;
module.exports.getType = getType;
module.exports.invalidSegmentRegEx = invalidSegmentRegEx;
module.exports.isInside = isInside;
module.exports.isRelativeRequest = isRelativeRequest;
module.exports.isSubPath = isSubPath;
module.exports.isWindowsPath = isWindowsPath;
module.exports.join = join;
module.exports.normalize = normalize;
module.exports.toPath = toPath;
