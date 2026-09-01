/*!
 * /**
 *  * Copyright (c) Meta Platforms, Inc. and affiliates.
 *  *
 *  * This source code is licensed under the MIT license found in the
 *  * LICENSE file in the root directory of this source tree.
 *  * /
 */
/******/ (() => { // webpackBootstrap
/******/ 	"use strict";
/******/ 	var __webpack_modules__ = ({

/***/ "./src/HasteFS.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
var _constants = _interopRequireDefault(__webpack_require__("./src/constants.ts"));
var fastPath = _interopRequireWildcard(__webpack_require__("./src/lib/fast_path.ts"));
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class HasteFS {
  _rootDir;
  _files;
  constructor({
    rootDir,
    files
  }) {
    this._rootDir = rootDir;
    this._files = files;
  }
  getModuleName(file) {
    const fileMetadata = this._getFileData(file);
    return fileMetadata && fileMetadata[_constants.default.ID] || null;
  }
  getSize(file) {
    const fileMetadata = this._getFileData(file);
    return fileMetadata && fileMetadata[_constants.default.SIZE] || null;
  }
  getDependencies(file) {
    const fileMetadata = this._getFileData(file);
    if (fileMetadata) {
      return fileMetadata[_constants.default.DEPENDENCIES] ? fileMetadata[_constants.default.DEPENDENCIES].split(_constants.default.DEPENDENCY_DELIM) : [];
    } else {
      return null;
    }
  }
  getSha1(file) {
    const fileMetadata = this._getFileData(file);
    return fileMetadata && fileMetadata[_constants.default.SHA1] || null;
  }
  exists(file) {
    return this._getFileData(file) != null;
  }
  getAllFiles() {
    return [...this.getAbsoluteFileIterator()];
  }
  getFileIterator() {
    return this._files.keys();
  }
  *getAbsoluteFileIterator() {
    for (const file of this.getFileIterator()) {
      yield fastPath.resolve(this._rootDir, file);
    }
  }
  matchFiles(pattern) {
    if (!(pattern instanceof RegExp)) {
      pattern = new RegExp(pattern);
    }
    const files = [];
    for (const file of this.getAbsoluteFileIterator()) {
      if (pattern.test(file)) {
        files.push(file);
      }
    }
    return files;
  }
  matchFilesWithGlob(globs, root) {
    const files = new Set();
    const matcher = (0, _jestUtil().globsToMatcher)(globs);
    for (const file of this.getAbsoluteFileIterator()) {
      const filePath = root ? fastPath.relative(root, file) : file;
      if (matcher((0, _jestUtil().replacePathSepForGlob)(filePath))) {
        files.add(file);
      }
    }
    return files;
  }
  _getFileData(file) {
    const relativePath = fastPath.relative(this._rootDir, file);
    return this._files.get(relativePath);
  }
}
exports["default"] = HasteFS;

/***/ },

/***/ "./src/ModuleMap.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _constants = _interopRequireDefault(__webpack_require__("./src/constants.ts"));
var fastPath = _interopRequireWildcard(__webpack_require__("./src/lib/fast_path.ts"));
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const EMPTY_OBJ = {};
const EMPTY_MAP = new Map();
class ModuleMap {
  static DuplicateHasteCandidatesError;
  _raw;
  json;
  static mapToArrayRecursive(map) {
    let arr = [...map];
    if (arr[0] && arr[0][1] instanceof Map) {
      arr = arr.map(el => [el[0], this.mapToArrayRecursive(el[1])]);
    }
    return arr;
  }
  static mapFromArrayRecursive(arr) {
    if (arr[0] && Array.isArray(arr[1])) {
      arr = arr.map(el => [el[0], this.mapFromArrayRecursive(el[1])]);
    }
    return new Map(arr);
  }
  constructor(raw) {
    this._raw = raw;
  }
  getModule(name, platform, supportsNativePlatform, type) {
    if (type == null) {
      type = _constants.default.MODULE;
    }
    const module = this._getModuleMetadata(name, platform, !!supportsNativePlatform);
    if (module && module[_constants.default.TYPE] === type) {
      const modulePath = module[_constants.default.PATH];
      return modulePath && fastPath.resolve(this._raw.rootDir, modulePath);
    }
    return null;
  }
  getPackage(name, platform, _supportsNativePlatform) {
    return this.getModule(name, platform, null, _constants.default.PACKAGE);
  }
  getMockModule(name) {
    const mockPath = this._raw.mocks.get(name) || this._raw.mocks.get(`${name}/index`);
    return mockPath && fastPath.resolve(this._raw.rootDir, mockPath);
  }
  getRawModuleMap() {
    return {
      duplicates: this._raw.duplicates,
      map: this._raw.map,
      mocks: this._raw.mocks,
      rootDir: this._raw.rootDir
    };
  }
  toJSON() {
    if (!this.json) {
      this.json = {
        duplicates: ModuleMap.mapToArrayRecursive(this._raw.duplicates),
        map: [...this._raw.map],
        mocks: [...this._raw.mocks],
        rootDir: this._raw.rootDir
      };
    }
    return this.json;
  }
  static fromJSON(serializableModuleMap) {
    return new ModuleMap({
      duplicates: ModuleMap.mapFromArrayRecursive(serializableModuleMap.duplicates),
      map: new Map(serializableModuleMap.map),
      mocks: new Map(serializableModuleMap.mocks),
      rootDir: serializableModuleMap.rootDir
    });
  }

  /**
   * When looking up a module's data, we walk through each eligible platform for
   * the query. For each platform, we want to check if there are known
   * duplicates for that name+platform pair. The duplication logic normally
   * removes elements from the `map` object, but we want to check upfront to be
   * extra sure. If metadata exists both in the `duplicates` object and the
   * `map`, this would be a bug.
   */
  _getModuleMetadata(name, platform, supportsNativePlatform) {
    const map = this._raw.map.get(name) || EMPTY_OBJ;
    const dupMap = this._raw.duplicates.get(name) || EMPTY_MAP;
    if (platform != null) {
      this._assertNoDuplicates(name, platform, supportsNativePlatform, dupMap.get(platform));
      if (map[platform] != null) {
        return map[platform];
      }
    }
    if (supportsNativePlatform) {
      this._assertNoDuplicates(name, _constants.default.NATIVE_PLATFORM, supportsNativePlatform, dupMap.get(_constants.default.NATIVE_PLATFORM));
      if (map[_constants.default.NATIVE_PLATFORM]) {
        return map[_constants.default.NATIVE_PLATFORM];
      }
    }
    this._assertNoDuplicates(name, _constants.default.GENERIC_PLATFORM, supportsNativePlatform, dupMap.get(_constants.default.GENERIC_PLATFORM));
    if (map[_constants.default.GENERIC_PLATFORM]) {
      return map[_constants.default.GENERIC_PLATFORM];
    }
    return null;
  }
  _assertNoDuplicates(name, platform, supportsNativePlatform, relativePathSet) {
    if (relativePathSet == null) {
      return;
    }
    // Force flow refinement
    const previousSet = relativePathSet;
    const duplicates = new Map();
    for (const [relativePath, type] of previousSet) {
      const duplicatePath = fastPath.resolve(this._raw.rootDir, relativePath);
      duplicates.set(duplicatePath, type);
    }
    throw new DuplicateHasteCandidatesError(name, platform, supportsNativePlatform, duplicates);
  }
  static create(rootDir) {
    return new ModuleMap({
      duplicates: new Map(),
      map: new Map(),
      mocks: new Map(),
      rootDir
    });
  }
}
exports["default"] = ModuleMap;
class DuplicateHasteCandidatesError extends Error {
  hasteName;
  platform;
  supportsNativePlatform;
  duplicatesSet;
  constructor(name, platform, supportsNativePlatform, duplicatesSet) {
    const platformMessage = getPlatformMessage(platform);
    super(`The name \`${name}\` was looked up in the Haste module map. It ` + 'cannot be resolved, because there exists several different ' + 'files, or packages, that provide a module for ' + `that particular name and platform. ${platformMessage} You must ` + `delete or exclude files until there remains only one of these:\n\n${[...duplicatesSet].map(([dupFilePath, dupFileType]) => `  * \`${dupFilePath}\` (${getTypeMessage(dupFileType)})\n`).sort().join('')}`);
    this.hasteName = name;
    this.platform = platform;
    this.supportsNativePlatform = supportsNativePlatform;
    this.duplicatesSet = duplicatesSet;
  }
}
function getPlatformMessage(platform) {
  if (platform === _constants.default.GENERIC_PLATFORM) {
    return 'The platform is generic (no extension).';
  }
  return `The platform extension is \`${platform}\`.`;
}
function getTypeMessage(type) {
  switch (type) {
    case _constants.default.MODULE:
      return 'module';
    case _constants.default.PACKAGE:
      return 'package';
  }
  return 'unknown';
}
ModuleMap.DuplicateHasteCandidatesError = DuplicateHasteCandidatesError;

/***/ },

/***/ "./src/constants.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/*
 * This file exports a set of constants that are used for Jest's haste map
 * serialization. On very large repositories, the haste map cache becomes very
 * large to the point where it is the largest overhead in starting up Jest.
 *
 * This constant key map allows to keep the map smaller without having to build
 * a custom serialization library.
 */

/* eslint-disable sort-keys */
const constants = {
  /* dependency serialization */
  DEPENDENCY_DELIM: '\0',
  /* file map attributes */
  ID: 0,
  MTIME: 1,
  SIZE: 2,
  VISITED: 3,
  DEPENDENCIES: 4,
  SHA1: 5,
  /* module map attributes */
  PATH: 0,
  TYPE: 1,
  /* module types */
  MODULE: 0,
  PACKAGE: 1,
  /* platforms */
  GENERIC_PLATFORM: 'g',
  NATIVE_PLATFORM: 'native'
};
/* eslint-enable */
var _default = exports["default"] = constants;

/***/ },

/***/ "./src/crawlers/node.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.nodeCrawl = nodeCrawl;
function _nodeChild_process() {
  const data = require("node:child_process");
  _nodeChild_process = function () {
    return data;
  };
  return data;
}
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
    return data;
  };
  return data;
}
var _constants = _interopRequireDefault(__webpack_require__("./src/constants.ts"));
var fastPath = _interopRequireWildcard(__webpack_require__("./src/lib/fast_path.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

async function hasNativeFindSupport(forceNodeFilesystemAPI) {
  if (forceNodeFilesystemAPI) {
    return false;
  }
  try {
    return await new Promise(resolve => {
      // Check the find binary supports the non-POSIX -iname parameter wrapped in parens.
      const args = ['.', '-type', 'f', '(', '-iname', '*.ts', '-o', '-iname', '*.js', ')'];
      const child = (0, _nodeChild_process().spawn)('find', args, {
        cwd: __dirname
      });
      child.on('error', () => {
        resolve(false);
      });
      child.on('exit', code => {
        resolve(code === 0);
      });
    });
  } catch {
    return false;
  }
}
function find(roots, extensions, ignore, enableSymlinks, callback) {
  const result = [];
  let activeCalls = 0;
  function search(directory) {
    activeCalls++;
    fs().readdir(directory, {
      withFileTypes: true
    }, (err, entries) => {
      activeCalls--;
      if (err) {
        if (activeCalls === 0) {
          callback(result);
        }
        return;
      }
      for (const entry of entries) {
        const file = path().join(directory, entry.name);
        if (ignore(file)) {
          continue;
        }
        if (entry.isSymbolicLink()) {
          continue;
        }
        if (entry.isDirectory()) {
          search(file);
          continue;
        }
        activeCalls++;
        const stat = enableSymlinks ? fs().stat : fs().lstat;
        stat(file, (err, stat) => {
          activeCalls--;

          // This logic is unnecessary for node > v10.10, but leaving it in
          // since we need it for backwards-compatibility still.
          if (!err && stat && !stat.isSymbolicLink()) {
            if (stat.isDirectory()) {
              search(file);
            } else {
              const ext = path().extname(file).slice(1);
              if (extensions.includes(ext)) {
                result.push([file, stat.mtime.getTime(), stat.size]);
              }
            }
          }
          if (activeCalls === 0) {
            callback(result);
          }
        });
      }
      if (activeCalls === 0) {
        callback(result);
      }
    });
  }
  if (roots.length > 0) {
    for (const root of roots) search(root);
  } else {
    callback(result);
  }
}
function findNative(roots, extensions, ignore, enableSymlinks, callback) {
  const args = [...roots];
  if (enableSymlinks) {
    args.push('(', '-type', 'f', '-o', '-type', 'l', ')');
  } else {
    args.push('-type', 'f');
  }
  if (extensions.length > 0) {
    args.push('(');
  }
  for (const [index, ext] of extensions.entries()) {
    if (index) {
      args.push('-o');
    }
    args.push('-iname', `*.${ext}`);
  }
  if (extensions.length > 0) {
    args.push(')');
  }
  const child = (0, _nodeChild_process().spawn)('find', args);
  let stdout = '';
  if (child.stdout === null) {
    throw new Error('stdout is null - this should never happen. Please open up an issue at https://github.com/jestjs/jest');
  }
  child.stdout.setEncoding('utf8');
  child.stdout.on('data', data => stdout += data);
  child.stdout.on('close', () => {
    const lines = stdout.trim().split('\n').filter(x => !ignore(x));
    const result = [];
    let count = lines.length;
    if (count) {
      for (const path of lines) {
        fs().stat(path, (err, stat) => {
          // Filter out symlinks that describe directories
          if (!err && stat && !stat.isDirectory()) {
            result.push([path, stat.mtime.getTime(), stat.size]);
          }
          if (--count === 0) {
            callback(result);
          }
        });
      }
    } else {
      callback([]);
    }
  });
}
async function nodeCrawl(options) {
  const {
    data,
    extensions,
    forceNodeFilesystemAPI,
    ignore,
    rootDir,
    enableSymlinks,
    roots
  } = options;
  const useNativeFind = await hasNativeFindSupport(forceNodeFilesystemAPI);
  return new Promise(resolve => {
    const callback = list => {
      const files = new Map();
      const removedFiles = new Map(data.files);
      for (const fileData of list) {
        const [filePath, mtime, size] = fileData;
        const relativeFilePath = fastPath.relative(rootDir, filePath);
        const existingFile = data.files.get(relativeFilePath);
        if (existingFile && existingFile[_constants.default.MTIME] === mtime) {
          files.set(relativeFilePath, existingFile);
        } else {
          // See ../constants.js; SHA-1 will always be null and fulfilled later.
          files.set(relativeFilePath, ['', mtime, size, 0, '', null]);
        }
        removedFiles.delete(relativeFilePath);
      }
      data.files = files;
      resolve({
        hasteMap: data,
        removedFiles
      });
    };
    if (useNativeFind) {
      findNative(roots, extensions, ignore, enableSymlinks, callback);
    } else {
      find(roots, extensions, ignore, enableSymlinks, callback);
    }
  });
}

/***/ },

/***/ "./src/crawlers/watchman.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.watchmanCrawl = watchmanCrawl;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function watchman() {
  const data = _interopRequireWildcard(require("fb-watchman"));
  watchman = function () {
    return data;
  };
  return data;
}
var _constants = _interopRequireDefault(__webpack_require__("./src/constants.ts"));
var fastPath = _interopRequireWildcard(__webpack_require__("./src/lib/fast_path.ts"));
var _normalizePathSep = _interopRequireDefault(__webpack_require__("./src/lib/normalizePathSep.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const watchmanURL = 'https://facebook.github.io/watchman/docs/troubleshooting';
function watchmanError(error) {
  error.message = `Watchman error: ${error.message.trim()}. Make sure watchman ` + `is running for this project. See ${watchmanURL}.`;
  return error;
}

/**
 * Wrap watchman capabilityCheck method as a promise.
 *
 * @param client watchman client
 * @param caps capabilities to verify
 * @returns a promise resolving to a list of verified capabilities
 */
async function capabilityCheck(client, caps) {
  return new Promise((resolve, reject) => {
    client.capabilityCheck(
    // @ts-expect-error: incorrectly typed
    caps, (error, response) => {
      if (error) {
        reject(error);
      } else {
        resolve(response);
      }
    });
  });
}
async function watchmanCrawl(options) {
  const fields = ['name', 'exists', 'mtime_ms', 'size'];
  const {
    data,
    extensions,
    ignore,
    rootDir,
    roots
  } = options;
  const defaultWatchExpression = ['allof', ['type', 'f']];
  const clocks = data.clocks;
  const client = new (watchman().Client)();

  // https://facebook.github.io/watchman/docs/capabilities.html
  // Check adds about ~28ms
  const capabilities = await capabilityCheck(client, {
    // If a required capability is missing then an error will be thrown,
    // we don't need this assertion, so using optional instead.
    optional: ['suffix-set']
  });
  if (capabilities?.capabilities['suffix-set']) {
    // If available, use the optimized `suffix-set` operation:
    // https://facebook.github.io/watchman/docs/expr/suffix.html#suffix-set
    defaultWatchExpression.push(['suffix', extensions]);
  } else {
    // Otherwise use the older and less optimal suffix tuple array
    defaultWatchExpression.push(['anyof', ...extensions.map(extension => ['suffix', extension])]);
  }
  let clientError;
  client.on('error', error => clientError = watchmanError(error));
  const cmd = (...args) => new Promise((resolve, reject) =>
  // @ts-expect-error: client is typed strictly, but incomplete
  client.command(args, (error, result) => error ? reject(watchmanError(error)) : resolve(result)));
  if (options.computeSha1) {
    const {
      capabilities
    } = await cmd('list-capabilities');
    if (capabilities.includes('field-content.sha1hex')) {
      fields.push('content.sha1hex');
    }
  }
  async function getWatchmanRoots(roots) {
    const watchmanRoots = new Map();
    await Promise.all(roots.map(async root => {
      const response = await cmd('watch-project', root);
      const existing = watchmanRoots.get(response.watch);
      // A root can only be filtered if it was never seen with a
      // relative_path before.
      const canBeFiltered = !existing || existing.length > 0;
      if (canBeFiltered) {
        if (response.relative_path) {
          watchmanRoots.set(response.watch, [...(existing || []), response.relative_path]);
        } else {
          // Make the filter directories an empty array to signal that this
          // root was already seen and needs to be watched for all files or
          // directories.
          watchmanRoots.set(response.watch, []);
        }
      }
    }));
    return watchmanRoots;
  }
  async function queryWatchmanForDirs(rootProjectDirMappings) {
    const results = new Map();
    let isFresh = false;
    await Promise.all([...rootProjectDirMappings].map(async ([root, directoryFilters]) => {
      const expression = [...defaultWatchExpression];
      const glob = [];
      if (directoryFilters.length > 0) {
        expression.push(['anyof', ...directoryFilters.map(dir => ['dirname', dir])]);
        for (const directory of directoryFilters) {
          for (const extension of extensions) {
            glob.push(`${directory}/**/*.${extension}`);
          }
        }
      } else {
        for (const extension of extensions) {
          glob.push(`**/*.${extension}`);
        }
      }

      // Jest is only going to store one type of clock; a string that
      // represents a local clock. However, the Watchman crawler supports
      // a second type of clock that can be written by automation outside of
      // Jest, called an "scm query", which fetches changed files based on
      // source control mergebases. The reason this is necessary is because
      // local clocks are not portable across systems, but scm queries are.
      // By using scm queries, we can create the haste map on a different
      // system and import it, transforming the clock into a local clock.
      const since = clocks.get(fastPath.relative(rootDir, root));
      const query = since === undefined ?
      // Use the `since` generator if we have a clock available
      {
        expression,
        fields,
        glob,
        glob_includedotfiles: true
      } :
      // Otherwise use the `glob` filter
      {
        expression,
        fields,
        since
      };
      const response = await cmd('query', root, query);
      if ('warning' in response) {
        console.warn('watchman warning:', response.warning);
      }

      // When a source-control query is used, we ignore the "is fresh"
      // response from Watchman because it will be true despite the query
      // being incremental.
      const isSourceControlQuery = typeof since !== 'string' && since?.scm?.['mergebase-with'] !== undefined;
      if (!isSourceControlQuery) {
        isFresh = isFresh || response.is_fresh_instance;
      }
      results.set(root, response);
    }));
    return {
      isFresh,
      results
    };
  }
  let files = data.files;
  let removedFiles = new Map();
  const changedFiles = new Map();
  let results;
  let isFresh = false;
  try {
    const watchmanRoots = await getWatchmanRoots(roots);
    const watchmanFileResults = await queryWatchmanForDirs(watchmanRoots);

    // Reset the file map if watchman was restarted and sends us a list of
    // files.
    if (watchmanFileResults.isFresh) {
      files = new Map();
      removedFiles = new Map(data.files);
      isFresh = true;
    }
    results = watchmanFileResults.results;
  } finally {
    client.end();
  }
  if (clientError) {
    throw clientError;
  }
  for (const [watchRoot, response] of results) {
    const fsRoot = (0, _normalizePathSep.default)(watchRoot);
    const relativeFsRoot = fastPath.relative(rootDir, fsRoot);
    clocks.set(relativeFsRoot,
    // Ensure we persist only the local clock.
    typeof response.clock === 'string' ? response.clock : response.clock.clock);
    for (const fileData of response.files) {
      const filePath = fsRoot + path().sep + (0, _normalizePathSep.default)(fileData.name);
      const relativeFilePath = fastPath.relative(rootDir, filePath);
      const existingFileData = data.files.get(relativeFilePath);

      // If watchman is fresh, the removed files map starts with all files
      // and we remove them as we verify they still exist.
      if (isFresh && existingFileData && fileData.exists) {
        removedFiles.delete(relativeFilePath);
      }
      if (!fileData.exists) {
        // No need to act on files that do not exist and were not tracked.
        if (existingFileData) {
          files.delete(relativeFilePath);

          // If watchman is not fresh, we will know what specific files were
          // deleted since we last ran and can track only those files.
          if (!isFresh) {
            removedFiles.set(relativeFilePath, existingFileData);
          }
        }
      } else if (!ignore(filePath)) {
        const mtime = typeof fileData.mtime_ms === 'number' ? fileData.mtime_ms : fileData.mtime_ms.toNumber();
        const size = fileData.size;
        let sha1hex = fileData['content.sha1hex'];
        if (typeof sha1hex !== 'string' || sha1hex.length !== 40) {
          sha1hex = undefined;
        }
        let nextData;
        if (existingFileData && existingFileData[_constants.default.MTIME] === mtime) {
          nextData = existingFileData;
        } else if (existingFileData && sha1hex && existingFileData[_constants.default.SHA1] === sha1hex) {
          nextData = [existingFileData[0], mtime, existingFileData[2], existingFileData[3], existingFileData[4], existingFileData[5]];
        } else {
          // See ../constants.ts
          nextData = ['', mtime, size, 0, '', sha1hex ?? null];
        }
        files.set(relativeFilePath, nextData);
        changedFiles.set(relativeFilePath, nextData);
      }
    }
  }
  data.files = files;
  return {
    changedFiles: isFresh ? undefined : changedFiles,
    hasteMap: data,
    removedFiles
  };
}

/***/ },

/***/ "./src/getMockName.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const MOCKS_PATTERN = `${path().sep}__mocks__${path().sep}`;
const getMockName = filePath => {
  const mockPath = filePath.split(MOCKS_PATTERN)[1];
  return mockPath.slice(0, mockPath.lastIndexOf(path().extname(mockPath))).replaceAll('\\', '/');
};
var _default = exports["default"] = getMockName;

/***/ },

/***/ "./src/lib/fast_path.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.relative = relative;
exports.resolve = resolve;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// rootDir and filename must be absolute paths (resolved)
function relative(rootDir, filename) {
  return filename.startsWith(rootDir + path().sep) ? filename.slice(rootDir.length + 1) : path().relative(rootDir, filename);
}
const INDIRECTION_FRAGMENT = `..${path().sep}`;

// rootDir must be an absolute path and relativeFilename must be simple
// (e.g.: foo/bar or ../foo/bar, but never ./foo or foo/../bar)
function resolve(rootDir, relativeFilename) {
  return relativeFilename.startsWith(INDIRECTION_FRAGMENT) ? path().resolve(rootDir, relativeFilename) : rootDir + path().sep + relativeFilename;
}

/***/ },

/***/ "./src/lib/getPlatformExtension.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getPlatformExtension;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const SUPPORTED_PLATFORM_EXTS = new Set(['android', 'ios', 'native', 'web']);

// Extract platform extension: index.ios.js -> ios
function getPlatformExtension(file, platforms) {
  const last = file.lastIndexOf('.');
  const secondToLast = file.lastIndexOf('.', last - 1);
  if (secondToLast === -1) {
    return null;
  }
  const platform = file.slice(secondToLast + 1, last);
  // If an overriding platform array is passed, check that first

  if (platforms && platforms.includes(platform)) {
    return platform;
  }
  return SUPPORTED_PLATFORM_EXTS.has(platform) ? platform : null;
}

/***/ },

/***/ "./src/lib/isWatchmanInstalled.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = isWatchmanInstalled;
function _nodeChild_process() {
  const data = require("node:child_process");
  _nodeChild_process = function () {
    return data;
  };
  return data;
}
function _nodeUtil() {
  const data = require("node:util");
  _nodeUtil = function () {
    return data;
  };
  return data;
}
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

async function isWatchmanInstalled() {
  try {
    await (0, _nodeUtil().promisify)(_nodeChild_process().execFile)('watchman', ['--version']);
    return true;
  } catch {
    return false;
  }
}

/***/ },

/***/ "./src/lib/normalizePathSep.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

let normalizePathSep;
if (path().sep === '/') {
  normalizePathSep = filePath => filePath;
} else {
  normalizePathSep = filePath => filePath.replaceAll('/', path().sep);
}
var _default = exports["default"] = normalizePathSep;

/***/ },

/***/ "./src/watchers/FSEventsWatcher.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.FSEventsWatcher = void 0;
function _nodeEvents() {
  const data = require("node:events");
  _nodeEvents = function () {
    return data;
  };
  return data;
}
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _anymatch() {
  const data = _interopRequireDefault(require("anymatch"));
  _anymatch = function () {
    return data;
  };
  return data;
}
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
    return data;
  };
  return data;
}
function _walker() {
  const data = _interopRequireDefault(require("walker"));
  _walker = function () {
    return data;
  };
  return data;
}
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

// @ts-expect-error -- no types

// eslint-disable-next-line @typescript-eslint/prefer-ts-expect-error, @typescript-eslint/ban-ts-comment
// @ts-ignore: this is for CI which runs linux and might not have this
let fsevents = null;
try {
  fsevents = require('fsevents');
} catch {
  // Optional dependency, only supported on Darwin.
}
const CHANGE_EVENT = 'change';
const DELETE_EVENT = 'delete';
const ADD_EVENT = 'add';
const ALL_EVENT = 'all';
/**
 * Export `FSEventsWatcher` class.
 * Watches `dir`.
 */
class FSEventsWatcher extends _nodeEvents().EventEmitter {
  root;
  ignored;
  glob;
  dot;
  hasIgnore;
  doIgnore;
  fsEventsWatchStopper;
  _tracked;
  static isSupported() {
    return fsevents !== null;
  }
  static normalizeProxy(callback) {
    return (filepath, stats) => callback(path().normalize(filepath), stats);
  }
  static recReaddir(dir, dirCallback, fileCallback, endCallback, errorCallback, ignored) {
    (0, _walker().default)(dir).filterDir(currentDir => !ignored || !(0, _anymatch().default)(ignored, currentDir)).on('dir', FSEventsWatcher.normalizeProxy(dirCallback)).on('file', FSEventsWatcher.normalizeProxy(fileCallback)).on('error', errorCallback).on('end', () => {
      endCallback();
    });
  }
  constructor(dir, opts) {
    if (!fsevents) {
      throw new Error('`fsevents` unavailable (this watcher can only be used on Darwin)');
    }
    super();
    this.dot = opts.dot || false;
    this.ignored = opts.ignored;
    this.glob = Array.isArray(opts.glob) ? opts.glob : [opts.glob];
    this.hasIgnore = Boolean(opts.ignored) && !(Array.isArray(opts) && opts.length > 0);
    this.doIgnore = opts.ignored ? (0, _anymatch().default)(opts.ignored) : () => false;
    this.root = path().resolve(dir);
    this.fsEventsWatchStopper = fsevents.watch(this.root, this.handleEvent.bind(this));
    this._tracked = new Set();
    FSEventsWatcher.recReaddir(this.root, filepath => {
      this._tracked.add(filepath);
    }, filepath => {
      this._tracked.add(filepath);
    }, this.emit.bind(this, 'ready'), this.emit.bind(this, 'error'), this.ignored);
  }

  /**
   * End watching.
   */
  async close(callback) {
    await this.fsEventsWatchStopper();
    this.removeAllListeners();
    if (typeof callback === 'function') {
      process.nextTick(() => callback());
    }
  }
  isFileIncluded(relativePath) {
    if (this.doIgnore(relativePath)) {
      return false;
    }
    return this.glob.length > 0 ? (0, _jestUtil().globsToMatcher)(this.glob, {
      dot: this.dot
    })(relativePath) : this.dot || (0, _jestUtil().globsToMatcher)(['**/*'])(relativePath);
  }
  handleEvent(filepath) {
    const relativePath = path().relative(this.root, filepath);
    if (!this.isFileIncluded(relativePath)) {
      return;
    }
    fs().lstat(filepath, (error, stat) => {
      if (error && error.code !== 'ENOENT') {
        this.emit('error', error);
        return;
      }
      if (error) {
        // Ignore files that aren't tracked and don't exist.
        if (!this._tracked.has(filepath)) {
          return;
        }
        this._emit(DELETE_EVENT, relativePath);
        this._tracked.delete(filepath);
        return;
      }
      if (this._tracked.has(filepath)) {
        this._emit(CHANGE_EVENT, relativePath, stat);
      } else {
        this._tracked.add(filepath);
        this._emit(ADD_EVENT, relativePath, stat);
      }
    });
  }

  /**
   * Emit events.
   */
  _emit(type, file, stat) {
    this.emit(type, file, this.root, stat);
    this.emit(ALL_EVENT, type, file, this.root, stat);
  }
}
exports.FSEventsWatcher = FSEventsWatcher;

/***/ },

/***/ "./src/watchers/NodeWatcher.js"
(module, __unused_webpack_exports, __webpack_require__) {

// vendored from https://github.com/amasad/sane/blob/64ff3a870c42e84f744086884bf55a4f9c22d376/src/node_watcher.js



const EventEmitter = (__webpack_require__("node:events").EventEmitter);
const fs = require('node:fs');
const platform = (__webpack_require__("node:os").platform)();
const path = require('node:path');
const common = __webpack_require__("./src/watchers/common.js");

/**
 * Constants
 */

const DEFAULT_DELAY = common.DEFAULT_DELAY;
const CHANGE_EVENT = common.CHANGE_EVENT;
const DELETE_EVENT = common.DELETE_EVENT;
const ADD_EVENT = common.ADD_EVENT;
const ALL_EVENT = common.ALL_EVENT;

/**
 * Export `NodeWatcher` class.
 * Watches `dir`.
 *
 * @class NodeWatcher
 * @param {String} dir
 * @param {Object} opts
 * @public
 */

module.exports = class NodeWatcher extends EventEmitter {
  constructor(dir, opts) {
    super();

    common.assignOptions(this, opts);

    this.watched = Object.create(null);
    this.changeTimers = Object.create(null);
    this.dirRegistry = Object.create(null);
    this.root = path.resolve(dir);
    this.watchdir = this.watchdir.bind(this);
    this.register = this.register.bind(this);
    this.checkedEmitError = this.checkedEmitError.bind(this);

    this.watchdir(this.root);
    common.recReaddir(
      this.root,
      this.watchdir,
      this.register,
      this.emit.bind(this, 'ready'),
      this.checkedEmitError,
      this.ignored,
    );
  }

  /**
   * Register files that matches our globs to know what to type of event to
   * emit in the future.
   *
   * Registry looks like the following:
   *
   *  dirRegister => Map {
   *    dirpath => Map {
   *       filename => true
   *    }
   *  }
   *
   * @param {string} filepath
   * @return {boolean} whether or not we have registered the file.
   * @private
   */

  register(filepath) {
    const relativePath = path.relative(this.root, filepath);
    if (
      !common.isFileIncluded(this.globs, this.dot, this.doIgnore, relativePath)
    ) {
      return false;
    }

    const dir = path.dirname(filepath);
    if (!this.dirRegistry[dir]) {
      this.dirRegistry[dir] = Object.create(null);
    }

    const filename = path.basename(filepath);
    this.dirRegistry[dir][filename] = true;

    return true;
  }

  /**
   * Removes a file from the registry.
   *
   * @param {string} filepath
   * @private
   */

  unregister(filepath) {
    const dir = path.dirname(filepath);
    if (this.dirRegistry[dir]) {
      const filename = path.basename(filepath);
      delete this.dirRegistry[dir][filename];
    }
  }

  /**
   * Removes a dir from the registry.
   *
   * @param {string} dirpath
   * @private
   */

  unregisterDir(dirpath) {
    if (this.dirRegistry[dirpath]) {
      delete this.dirRegistry[dirpath];
    }
  }

  /**
   * Checks if a file or directory exists in the registry.
   *
   * @param {string} fullpath
   * @return {boolean}
   * @private
   */

  registered(fullpath) {
    const dir = path.dirname(fullpath);
    return (
      this.dirRegistry[fullpath] ||
      (this.dirRegistry[dir] && this.dirRegistry[dir][path.basename(fullpath)])
    );
  }

  /**
   * Emit "error" event if it's not an ignorable event
   *
   * @param error
   * @private
   */
  checkedEmitError(error) {
    if (!isIgnorableFileError(error)) {
      this.emit('error', error);
    }
  }

  /**
   * Watch a directory.
   *
   * @param {string} dir
   * @private
   */

  watchdir(dir) {
    if (this.watched[dir]) {
      return;
    }

    const watcher = fs.watch(
      dir,
      {persistent: true},
      this.normalizeChange.bind(this, dir),
    );
    this.watched[dir] = watcher;

    watcher.on('error', this.checkedEmitError);

    if (this.root !== dir) {
      this.register(dir);
    }
  }

  /**
   * Stop watching a directory.
   *
   * @param {string} dir
   * @private
   */

  stopWatching(dir) {
    if (this.watched[dir]) {
      this.watched[dir].close();
      delete this.watched[dir];
    }
  }

  /**
   * End watching.
   *
   * @public
   */

  close() {
    for (const key of Object.keys(this.watched)) this.stopWatching(key);
    this.removeAllListeners();

    return Promise.resolve();
  }

  /**
   * On some platforms, as pointed out on the fs docs (most likely just win32)
   * the file argument might be missing from the fs event. Try to detect what
   * change by detecting if something was deleted or the most recent file change.
   *
   * @param {string} dir
   * @param {string} event
   * @param {string} file
   * @public
   */

  detectChangedFile(dir, event, callback) {
    if (!this.dirRegistry[dir]) {
      return;
    }

    let found = false;
    let closest = {mtime: 0};
    let c = 0;
    // eslint-disable-next-line unicorn/no-array-for-each
    Object.keys(this.dirRegistry[dir]).forEach((file, i, arr) => {
      fs.lstat(path.join(dir, file), (error, stat) => {
        if (found) {
          return;
        }

        if (error) {
          if (isIgnorableFileError(error)) {
            found = true;
            callback(file);
          } else {
            this.emit('error', error);
          }
        } else {
          if (stat.mtime > closest.mtime) {
            stat.file = file;
            closest = stat;
          }
          if (arr.length === ++c) {
            callback(closest.file);
          }
        }
      });
    });
  }

  /**
   * Normalize fs events and pass it on to be processed.
   *
   * @param {string} dir
   * @param {string} event
   * @param {string} file
   * @public
   */

  normalizeChange(dir, event, file) {
    if (file) {
      this.processChange(dir, event, path.normalize(file));
    } else {
      this.detectChangedFile(dir, event, actualFile => {
        if (actualFile) {
          this.processChange(dir, event, actualFile);
        }
      });
    }
  }

  /**
   * Process changes.
   *
   * @param {string} dir
   * @param {string} event
   * @param {string} file
   * @public
   */

  processChange(dir, event, file) {
    const fullPath = path.join(dir, file);
    const relativePath = path.join(path.relative(this.root, dir), file);

    fs.lstat(fullPath, (error, stat) => {
      if (error && error.code !== 'ENOENT') {
        this.emit('error', error);
      } else if (!error && stat.isDirectory()) {
        // win32 emits usless change events on dirs.
        if (event !== 'change') {
          this.watchdir(fullPath);
          if (
            common.isFileIncluded(
              this.globs,
              this.dot,
              this.doIgnore,
              relativePath,
            )
          ) {
            this.emitEvent(ADD_EVENT, relativePath, stat);
          }
        }
      } else {
        const registered = this.registered(fullPath);
        if (error && error.code === 'ENOENT') {
          this.unregister(fullPath);
          this.stopWatching(fullPath);
          this.unregisterDir(fullPath);
          if (registered) {
            this.emitEvent(DELETE_EVENT, relativePath);
          }
        } else if (registered) {
          this.emitEvent(CHANGE_EVENT, relativePath, stat);
        } else {
          if (this.register(fullPath)) {
            this.emitEvent(ADD_EVENT, relativePath, stat);
          }
        }
      }
    });
  }

  /**
   * Triggers a 'change' event after debounding it to take care of duplicate
   * events on os x.
   *
   * @private
   */

  emitEvent(type, file, stat) {
    const key = `${type}-${file}`;
    const addKey = `${ADD_EVENT}-${file}`;
    if (type === CHANGE_EVENT && this.changeTimers[addKey]) {
      // Ignore the change event that is immediately fired after an add event.
      // (This happens on Linux).
      return;
    }
    clearTimeout(this.changeTimers[key]);
    this.changeTimers[key] = setTimeout(() => {
      delete this.changeTimers[key];
      if (type === ADD_EVENT && stat.isDirectory()) {
        // Recursively emit add events and watch for sub-files/folders
        common.recReaddir(
          path.resolve(this.root, file),
          function emitAddDir(dir, stats) {
            this.watchdir(dir);
            this.rawEmitEvent(ADD_EVENT, path.relative(this.root, dir), stats);
          }.bind(this),
          function emitAddFile(file, stats) {
            this.register(file);
            this.rawEmitEvent(ADD_EVENT, path.relative(this.root, file), stats);
          }.bind(this),
          function endCallback() {},
          this.checkedEmitError,
          this.ignored,
        );
      } else {
        this.rawEmitEvent(type, file, stat);
      }
    }, DEFAULT_DELAY);
  }

  /**
   * Actually emit the events
   */
  rawEmitEvent(type, file, stat) {
    this.emit(type, file, this.root, stat);
    this.emit(ALL_EVENT, type, file, this.root, stat);
  }
};
/**
 * Determine if a given FS error can be ignored
 *
 * @private
 */
function isIgnorableFileError(error) {
  return (
    error.code === 'ENOENT' ||
    // Workaround Windows node issue #4337.
    (error.code === 'EPERM' && platform === 'win32')
  );
}


/***/ },

/***/ "./src/watchers/RecrawlWarning.js"
(module) {

// vendored from https://github.com/amasad/sane/blob/64ff3a870c42e84f744086884bf55a4f9c22d376/src/utils/recrawl-warning-dedupe.js



class RecrawlWarning {
  constructor(root, count) {
    this.root = root;
    this.count = count;
  }

  static findByRoot(root) {
    for (let i = 0; i < this.RECRAWL_WARNINGS.length; i++) {
      const warning = this.RECRAWL_WARNINGS[i];
      if (warning.root === root) {
        return warning;
      }
    }

    return undefined;
  }

  static isRecrawlWarningDupe(warningMessage) {
    if (typeof warningMessage !== 'string') {
      return false;
    }
    const match = warningMessage.match(this.REGEXP);
    if (!match) {
      return false;
    }
    const count = Number(match[1]);
    const root = match[2];

    const warning = this.findByRoot(root);

    if (warning) {
      // only keep the highest count, assume count to either stay the same or
      // increase.
      if (warning.count >= count) {
        return true;
      } else {
        // update the existing warning to the latest (highest) count
        warning.count = count;
        return false;
      }
    } else {
      this.RECRAWL_WARNINGS.push(new RecrawlWarning(root, count));
      return false;
    }
  }
}

RecrawlWarning.RECRAWL_WARNINGS = [];
RecrawlWarning.REGEXP =
  /Recrawled this watch (\d+) times, most recently because:\n([^:]+)/;

module.exports = RecrawlWarning;


/***/ },

/***/ "./src/watchers/WatchmanWatcher.js"
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ WatchmanWatcher)
});

;// external "node:assert"
const external_node_assert_namespaceObject = require("node:assert");
// EXTERNAL MODULE: external "node:events"
var external_node_events_ = __webpack_require__("node:events");
;// external "node:path"
const external_node_path_namespaceObject = require("node:path");
;// external "fb-watchman"
const external_fb_watchman_namespaceObject = require("fb-watchman");
var external_fb_watchman_default = /*#__PURE__*/__webpack_require__.n(external_fb_watchman_namespaceObject);
;// external "graceful-fs"
const external_graceful_fs_namespaceObject = require("graceful-fs");
var external_graceful_fs_default = /*#__PURE__*/__webpack_require__.n(external_graceful_fs_namespaceObject);
// EXTERNAL MODULE: ./src/watchers/RecrawlWarning.js
var RecrawlWarning = __webpack_require__("./src/watchers/RecrawlWarning.js");
var RecrawlWarning_default = /*#__PURE__*/__webpack_require__.n(RecrawlWarning);
// EXTERNAL MODULE: ./src/watchers/common.js
var common = __webpack_require__("./src/watchers/common.js");
;// ./src/watchers/WatchmanWatcher.js
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */









const CHANGE_EVENT = common.CHANGE_EVENT;
const DELETE_EVENT = common.DELETE_EVENT;
const ADD_EVENT = common.ADD_EVENT;
const ALL_EVENT = common.ALL_EVENT;
const SUB_NAME = 'sane-sub';

/**
 * Watches `dir`.
 *
 * @class PollWatcher
 * @param String dir
 * @param {Object} opts
 * @public
 */

function WatchmanWatcher(dir, opts) {
  common.assignOptions(this, opts);
  this.root = external_node_path_namespaceObject.resolve(dir);
  this.init();
}

Object.setPrototypeOf(WatchmanWatcher.prototype, external_node_events_.EventEmitter.prototype);

/**
 * Run the watchman `watch` command on the root and subscribe to changes.
 *
 * @private
 */

WatchmanWatcher.prototype.init = function () {
  if (this.client) {
    this.client.removeAllListeners();
  }

  const self = this;
  this.client = new (external_fb_watchman_default()).Client();
  this.client.on('error', error => {
    self.emit('error', error);
  });
  this.client.on('subscription', this.handleChangeEvent.bind(this));
  this.client.on('end', () => {
    console.warn('[sane] Warning: Lost connection to watchman, reconnecting..');
    self.init();
  });

  this.watchProjectInfo = null;

  function getWatchRoot() {
    return self.watchProjectInfo ? self.watchProjectInfo.root : self.root;
  }

  function onCapability(error, resp) {
    if (handleError(self, error)) {
      // The Watchman watcher is unusable on this system, we cannot continue
      return;
    }

    handleWarning(resp);

    self.capabilities = resp.capabilities;

    if (self.capabilities.relative_root) {
      self.client.command(['watch-project', getWatchRoot()], onWatchProject);
    } else {
      self.client.command(['watch', getWatchRoot()], onWatch);
    }
  }

  function onWatchProject(error, resp) {
    if (handleError(self, error)) {
      return;
    }

    handleWarning(resp);

    self.watchProjectInfo = {
      relativePath: resp.relative_path ?? '',
      root: resp.watch,
    };

    self.client.command(['clock', getWatchRoot()], onClock);
  }

  function onWatch(error, resp) {
    if (handleError(self, error)) {
      return;
    }

    handleWarning(resp);

    self.client.command(['clock', getWatchRoot()], onClock);
  }

  function onClock(error, resp) {
    if (handleError(self, error)) {
      return;
    }

    handleWarning(resp);

    const options = {
      fields: ['name', 'exists', 'new'],
      since: resp.clock,
    };

    // If the server has the wildmatch capability available it supports
    // the recursive **/*.foo style match and we can offload our globs
    // to the watchman server.  This saves both on data size to be
    // communicated back to us and compute for evaluating the globs
    // in our node process.
    if (self.capabilities.wildmatch) {
      if (self.globs.length === 0) {
        if (!self.dot) {
          // Make sure we honor the dot option if even we're not using globs.
          options.expression = [
            'match',
            '**',
            'wholename',
            {
              includedotfiles: false,
            },
          ];
        }
      } else {
        options.expression = ['anyof'];
        for (const i in self.globs) {
          options.expression.push([
            'match',
            self.globs[i],
            'wholename',
            {
              includedotfiles: self.dot,
            },
          ]);
        }
      }
    }

    if (self.capabilities.relative_root) {
      options.relative_root = self.watchProjectInfo.relativePath;
    }

    self.client.command(
      ['subscribe', getWatchRoot(), SUB_NAME, options],
      onSubscribe,
    );
  }

  function onSubscribe(error, resp) {
    if (handleError(self, error)) {
      return;
    }

    handleWarning(resp);

    self.emit('ready');
  }

  self.client.capabilityCheck(
    {
      optional: ['wildmatch', 'relative_root'],
    },
    onCapability,
  );
};

/**
 * Handles a change event coming from the subscription.
 *
 * @param {Object} resp
 * @private
 */

WatchmanWatcher.prototype.handleChangeEvent = function (resp) {
  external_node_assert_namespaceObject.strict.equal(resp.subscription, SUB_NAME, 'Invalid subscription event.');
  if (resp.is_fresh_instance) {
    this.emit('fresh_instance');
  }
  if (resp.is_fresh_instance) {
    this.emit('fresh_instance');
  }
  if (Array.isArray(resp.files)) {
    for (const file of resp.files) this.handleFileChange(file);
  }
};

/**
 * Handles a single change event record.
 *
 * @param {Object} changeDescriptor
 * @private
 */

WatchmanWatcher.prototype.handleFileChange = function (changeDescriptor) {
  const self = this;
  let absPath;
  let relativePath;

  if (this.capabilities.relative_root) {
    relativePath = changeDescriptor.name;
    absPath = external_node_path_namespaceObject.join(
      this.watchProjectInfo.root,
      this.watchProjectInfo.relativePath,
      relativePath,
    );
  } else {
    absPath = external_node_path_namespaceObject.join(this.root, changeDescriptor.name);
    relativePath = changeDescriptor.name;
  }

  if (
    !(self.capabilities.wildmatch && !this.hasIgnore) &&
    !common.isFileIncluded(this.globs, this.dot, this.doIgnore, relativePath)
  ) {
    return;
  }

  if (changeDescriptor.exists) {
    external_graceful_fs_default().lstat(absPath, (error, stat) => {
      // Files can be deleted between the event and the lstat call
      // the most reliable thing to do here is to ignore the event.
      if (error && error.code === 'ENOENT') {
        return;
      }

      if (handleError(self, error)) {
        return;
      }

      const eventType = changeDescriptor.new ? ADD_EVENT : CHANGE_EVENT;

      // Change event on dirs are mostly useless.
      if (!(eventType === CHANGE_EVENT && stat.isDirectory())) {
        self.emitEvent(eventType, relativePath, self.root, stat);
      }
    });
  } else {
    self.emitEvent(DELETE_EVENT, relativePath, self.root);
  }
};

/**
 * Dispatches the event.
 *
 * @param {string} eventType
 * @param {string} filepath
 * @param {string} root
 * @param {fs.Stat} stat
 * @private
 */

WatchmanWatcher.prototype.emitEvent = function (
  eventType,
  filepath,
  root,
  stat,
) {
  this.emit(eventType, filepath, root, stat);
  this.emit(ALL_EVENT, eventType, filepath, root, stat);
};

/**
 * Closes the watcher.
 *
 */

WatchmanWatcher.prototype.close = function () {
  this.client.removeAllListeners();
  this.client.end();
  return Promise.resolve();
};

/**
 * Handles an error and returns true if exists.
 *
 * @param {WatchmanWatcher} self
 * @param {Error} error
 * @private
 */

function handleError(self, error) {
  if (error == null) {
    return false;
  } else {
    self.emit('error', error);
    return true;
  }
}

/**
 * Handles a warning in the watchman resp object.
 *
 * @param {object} resp
 * @private
 */

function handleWarning(resp) {
  if ('warning' in resp) {
    if (RecrawlWarning_default().isRecrawlWarningDupe(resp.warning)) {
      return true;
    }
    console.warn(resp.warning);
    return true;
  } else {
    return false;
  }
}


/***/ },

/***/ "./src/watchers/common.js"
(__unused_webpack_module, exports, __webpack_require__) {

// vendored from https://github.com/amasad/sane/blob/64ff3a870c42e84f744086884bf55a4f9c22d376/src/common.js



const platform = (__webpack_require__("node:os").platform)();
const path = require('node:path');
const anymatch = require('anymatch');
const picomatch = require('picomatch');
const walker = require('walker');

/**
 * Constants
 */

exports.DEFAULT_DELAY = 100;
exports.CHANGE_EVENT = 'change';
exports.DELETE_EVENT = 'delete';
exports.ADD_EVENT = 'add';
exports.ALL_EVENT = 'all';

/**
 * Assigns options to the watcher.
 *
 * @param {NodeWatcher|PollWatcher|WatchmanWatcher} watcher
 * @param {?object} opts
 * @return {boolean}
 * @public
 */

exports.assignOptions = function (watcher, opts) {
  opts = opts || {};
  watcher.globs = opts.glob || [];
  watcher.dot = opts.dot || false;
  watcher.ignored = opts.ignored || false;

  if (!Array.isArray(watcher.globs)) {
    watcher.globs = [watcher.globs];
  }
  watcher.hasIgnore =
    Boolean(opts.ignored) && !(Array.isArray(opts) && opts.length > 0);
  watcher.doIgnore = opts.ignored ? anymatch(opts.ignored) : () => false;

  if (opts.watchman && opts.watchmanPath) {
    watcher.watchmanPath = opts.watchmanPath;
  }

  return opts;
};

/**
 * Checks a file relative path against the globs array.
 *
 * @param {array} globs
 * @param {string} relativePath
 * @return {boolean}
 * @public
 */

exports.isFileIncluded = function (globs, dot, doIgnore, relativePath) {
  if (doIgnore(relativePath)) {
    return false;
  }
  return globs.length > 0
    ? globs.some(glob => picomatch(glob, {dot})(relativePath))
    : dot || picomatch('**/*')(relativePath);
};

/**
 * Traverse a directory recursively calling `callback` on every directory.
 *
 * @param {string} dir
 * @param {function} dirCallback
 * @param {function} fileCallback
 * @param {function} endCallback
 * @param {*} ignored
 * @public
 */

exports.recReaddir = function (
  dir,
  dirCallback,
  fileCallback,
  endCallback,
  errorCallback,
  ignored,
) {
  walker(dir)
    .filterDir(currentDir => !anymatch(ignored, currentDir))
    .on('dir', normalizeProxy(dirCallback))
    .on('file', normalizeProxy(fileCallback))
    .on('error', errorCallback)
    .on('end', () => {
      if (platform === 'win32') {
        setTimeout(endCallback, 1000);
      } else {
        endCallback();
      }
    });
};

/**
 * Returns a callback that when called will normalize a path and call the
 * original callback
 *
 * @param {function} callback
 * @return {function}
 * @private
 */

function normalizeProxy(callback) {
  return (filepath, stats) => callback(path.normalize(filepath), stats);
}


/***/ },

/***/ "node:events"
(module) {

module.exports = require("node:events");

/***/ },

/***/ "node:os"
(module) {

module.exports = require("node:os");

/***/ },

/***/ "./package.json"
(module) {

module.exports = {"version":"30.4.0"};

/***/ }

/******/ 	});
/************************************************************************/
/******/ 	// The module cache
/******/ 	var __webpack_module_cache__ = {};
/******/ 	
/******/ 	// The require function
/******/ 	function __webpack_require__(moduleId) {
/******/ 		// Check if module is in cache
/******/ 		var cachedModule = __webpack_module_cache__[moduleId];
/******/ 		if (cachedModule !== undefined) {
/******/ 			return cachedModule.exports;
/******/ 		}
/******/ 		// Create a new module (and put it into the cache)
/******/ 		var module = __webpack_module_cache__[moduleId] = {
/******/ 			// no module.id needed
/******/ 			// no module.loaded needed
/******/ 			exports: {}
/******/ 		};
/******/ 	
/******/ 		// Execute the module function
/******/ 		__webpack_modules__[moduleId](module, module.exports, __webpack_require__);
/******/ 	
/******/ 		// Return the exports of the module
/******/ 		return module.exports;
/******/ 	}
/******/ 	
/************************************************************************/
/******/ 	/* webpack/runtime/compat get default export */
/******/ 	(() => {
/******/ 		// getDefaultExport function for compatibility with non-harmony modules
/******/ 		__webpack_require__.n = (module) => {
/******/ 			var getter = module && module.__esModule ?
/******/ 				() => (module['default']) :
/******/ 				() => (module);
/******/ 			__webpack_require__.d(getter, { a: getter });
/******/ 			return getter;
/******/ 		};
/******/ 	})();
/******/ 	
/******/ 	/* webpack/runtime/define property getters */
/******/ 	(() => {
/******/ 		// define getter functions for harmony exports
/******/ 		__webpack_require__.d = (exports, definition) => {
/******/ 			for(var key in definition) {
/******/ 				if(__webpack_require__.o(definition, key) && !__webpack_require__.o(exports, key)) {
/******/ 					Object.defineProperty(exports, key, { enumerable: true, get: definition[key] });
/******/ 				}
/******/ 			}
/******/ 		};
/******/ 	})();
/******/ 	
/******/ 	/* webpack/runtime/hasOwnProperty shorthand */
/******/ 	(() => {
/******/ 		__webpack_require__.o = (obj, prop) => (Object.prototype.hasOwnProperty.call(obj, prop))
/******/ 	})();
/******/ 	
/******/ 	/* webpack/runtime/make namespace object */
/******/ 	(() => {
/******/ 		// define __esModule on exports
/******/ 		__webpack_require__.r = (exports) => {
/******/ 			if(typeof Symbol !== 'undefined' && Symbol.toStringTag) {
/******/ 				Object.defineProperty(exports, Symbol.toStringTag, { value: 'Module' });
/******/ 			}
/******/ 			Object.defineProperty(exports, '__esModule', { value: true });
/******/ 		};
/******/ 	})();
/******/ 	
/************************************************************************/
var __webpack_exports__ = {};
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = exports.ModuleMap = exports.DuplicateError = void 0;
function _nodeCrypto() {
  const data = require("node:crypto");
  _nodeCrypto = function () {
    return data;
  };
  return data;
}
function _nodeEvents() {
  const data = require("node:events");
  _nodeEvents = function () {
    return data;
  };
  return data;
}
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
    return data;
  };
  return data;
}
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _nodeV() {
  const data = require("node:v8");
  _nodeV = function () {
    return data;
  };
  return data;
}
function _gracefulFs() {
  const data = require("graceful-fs");
  _gracefulFs = function () {
    return data;
  };
  return data;
}
function _jestRegexUtil() {
  const data = require("jest-regex-util");
  _jestRegexUtil = function () {
    return data;
  };
  return data;
}
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
function _jestWorker() {
  const data = require("jest-worker");
  _jestWorker = function () {
    return data;
  };
  return data;
}
var _HasteFS = _interopRequireDefault(__webpack_require__("./src/HasteFS.ts"));
var _ModuleMap = _interopRequireDefault(__webpack_require__("./src/ModuleMap.ts"));
var _constants = _interopRequireDefault(__webpack_require__("./src/constants.ts"));
var _node = __webpack_require__("./src/crawlers/node.ts");
var _watchman = __webpack_require__("./src/crawlers/watchman.ts");
var _getMockName = _interopRequireDefault(__webpack_require__("./src/getMockName.ts"));
var fastPath = _interopRequireWildcard(__webpack_require__("./src/lib/fast_path.ts"));
var _getPlatformExtension = _interopRequireDefault(__webpack_require__("./src/lib/getPlatformExtension.ts"));
var _isWatchmanInstalled = _interopRequireDefault(__webpack_require__("./src/lib/isWatchmanInstalled.ts"));
var _normalizePathSep = _interopRequireDefault(__webpack_require__("./src/lib/normalizePathSep.ts"));
var _FSEventsWatcher = __webpack_require__("./src/watchers/FSEventsWatcher.ts");
var _NodeWatcher = _interopRequireDefault(__webpack_require__("./src/watchers/NodeWatcher.js"));
var _WatchmanWatcher = _interopRequireDefault(__webpack_require__("./src/watchers/WatchmanWatcher.js"));
var _worker = require("./worker");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// @ts-expect-error: not converted to TypeScript - it's a fork: https://github.com/jestjs/jest/pull/10919

// @ts-expect-error: not converted to TypeScript - it's a fork: https://github.com/jestjs/jest/pull/5387

// TypeScript doesn't like us importing from outside `rootDir`, but it doesn't
// understand `require`.
const {
  version: VERSION
} = __webpack_require__("./package.json");
let isWatchmanInstalledPromise;
const ModuleMap = exports.ModuleMap = _ModuleMap.default;
const CHANGE_INTERVAL = 30;
const MAX_WAIT_TIME = 240_000;
const NODE_MODULES = `${path().sep}node_modules${path().sep}`;
const PACKAGE_JSON = `${path().sep}package.json`;
const VCS_DIRECTORIES = ['.git', '.hg', '.sl'].map(vcs => (0, _jestRegexUtil().escapePathForRegex)(path().sep + vcs + path().sep)).join('|');
/**
 * HasteMap is a JavaScript implementation of Facebook's haste module system.
 *
 * This implementation is inspired by https://github.com/facebook/node-haste
 * and was built with for high-performance in large code repositories with
 * hundreds of thousands of files. This implementation is scalable and provides
 * predictable performance.
 *
 * Because the haste map creation and synchronization is critical to startup
 * performance and most tasks are blocked by I/O this class makes heavy use of
 * synchronous operations. It uses worker processes for parallelizing file
 * access and metadata extraction.
 *
 * The data structures created by `jest-haste-map` can be used directly from the
 * cache without further processing. The metadata objects in the `files` and
 * `map` objects contain cross-references: a metadata object from one can look
 * up the corresponding metadata object in the other map. Note that in most
 * projects, the number of files will be greater than the number of haste
 * modules one module can refer to many files based on platform extensions.
 *
 * type HasteMap = {
 *   clocks: WatchmanClocks,
 *   files: {[filepath: string]: FileMetaData},
 *   map: {[id: string]: ModuleMapItem},
 *   mocks: {[id: string]: string},
 * }
 *
 * // Watchman clocks are used for query synchronization and file system deltas.
 * type WatchmanClocks = {[filepath: string]: string};
 *
 * type FileMetaData = {
 *   id: ?string, // used to look up module metadata objects in `map`.
 *   mtime: number, // check for outdated files.
 *   size: number, // size of the file in bytes.
 *   visited: boolean, // whether the file has been parsed or not.
 *   dependencies: Array<string>, // all relative dependencies of this file.
 *   sha1: ?string, // SHA-1 of the file, if requested via options.
 * };
 *
 * // Modules can be targeted to a specific platform based on the file name.
 * // Example: platform.ios.js and Platform.android.js will both map to the same
 * // `Platform` module. The platform should be specified during resolution.
 * type ModuleMapItem = {[platform: string]: ModuleMetaData};
 *
 * //
 * type ModuleMetaData = {
 *   path: string, // the path to look up the file object in `files`.
 *   type: string, // the module type (either `package` or `module`).
 * };
 *
 * Note that the data structures described above are conceptual only. The actual
 * implementation uses arrays and constant keys for metadata storage. Instead of
 * `{id: 'flatMap', mtime: 3421, size: 42, visited: true, dependencies: []}` the real
 * representation is similar to `['flatMap', 3421, 42, 1, []]` to save storage space
 * and reduce parse and write time of a big JSON blob.
 *
 * The HasteMap is created as follows:
 *  1. read data from the cache or create an empty structure.
 *
 *  2. crawl the file system.
 *     * empty cache: crawl the entire file system.
 *     * cache available:
 *       * if watchman is available: get file system delta changes.
 *       * if watchman is unavailable: crawl the entire file system.
 *     * build metadata objects for every file. This builds the `files` part of
 *       the `HasteMap`.
 *
 *  3. parse and extract metadata from changed files.
 *     * this is done in parallel over worker processes to improve performance.
 *     * the worst case is to parse all files.
 *     * the best case is no file system access and retrieving all data from
 *       the cache.
 *     * the average case is a small number of changed files.
 *
 *  4. serialize the new `HasteMap` in a cache file.
 *     Worker processes can directly access the cache through `HasteMap.read()`.
 *
 */
class HasteMap extends _nodeEvents().EventEmitter {
  _buildPromise = null;
  _cachePath = '';
  _changeInterval;
  _console;
  _options;
  _watchers = [];
  _worker = null;
  static getStatic(config) {
    if (config.haste.hasteMapModulePath) {
      return require(config.haste.hasteMapModulePath);
    }
    return HasteMap;
  }
  static async create(options) {
    if (options.hasteMapModulePath) {
      const CustomHasteMap = require(options.hasteMapModulePath);
      return new CustomHasteMap(options);
    }
    const hasteMap = new HasteMap(options);
    await hasteMap.setupCachePath(options);
    return hasteMap;
  }
  constructor(options) {
    super();
    this._options = {
      cacheDirectory: options.cacheDirectory || (0, _nodeOs().tmpdir)(),
      computeDependencies: options.computeDependencies ?? true,
      computeSha1: options.computeSha1 || false,
      dependencyExtractor: options.dependencyExtractor || null,
      enableSymlinks: options.enableSymlinks || false,
      extensions: options.extensions,
      forceNodeFilesystemAPI: !!options.forceNodeFilesystemAPI,
      hasteImplModulePath: options.hasteImplModulePath,
      id: options.id,
      maxWorkers: options.maxWorkers,
      mocksPattern: options.mocksPattern ? new RegExp(options.mocksPattern) : null,
      platforms: options.platforms,
      resetCache: options.resetCache,
      retainAllFiles: options.retainAllFiles,
      rootDir: options.rootDir,
      roots: [...new Set(options.roots)],
      skipPackageJson: !!options.skipPackageJson,
      throwOnModuleCollision: !!options.throwOnModuleCollision,
      useWatchman: options.useWatchman ?? true,
      watch: !!options.watch,
      workerThreads: options.workerThreads
    };
    this._console = options.console || globalThis.console;
    if (options.ignorePattern) {
      if (options.ignorePattern instanceof RegExp) {
        this._options.ignorePattern = new RegExp(`${options.ignorePattern.source}|${VCS_DIRECTORIES}`, options.ignorePattern.flags);
      } else {
        throw new TypeError('jest-haste-map: the `ignorePattern` option must be a RegExp');
      }
    } else {
      this._options.ignorePattern = new RegExp(VCS_DIRECTORIES);
    }
    if (this._options.enableSymlinks && this._options.useWatchman) {
      throw new Error('jest-haste-map: enableSymlinks config option was set, but ' + 'is incompatible with watchman.\n' + 'Set either `enableSymlinks` to false or `useWatchman` to false.');
    }
  }
  async setupCachePath(options) {
    const rootDirHash = (0, _nodeCrypto().createHash)('sha1').update(options.rootDir).digest('hex').slice(0, 32);
    let hasteImplHash = '';
    let dependencyExtractorHash = '';
    if (options.hasteImplModulePath) {
      const hasteImpl = require(options.hasteImplModulePath);
      if (hasteImpl.getCacheKey) {
        hasteImplHash = String(hasteImpl.getCacheKey());
      }
    }
    if (options.dependencyExtractor) {
      const dependencyExtractor = await (0, _jestUtil().requireOrImportModule)(options.dependencyExtractor, false);
      if (dependencyExtractor.getCacheKey) {
        dependencyExtractorHash = String(dependencyExtractor.getCacheKey());
      }
    }
    this._cachePath = HasteMap.getCacheFilePath(this._options.cacheDirectory, `haste-map-${this._options.id}-${rootDirHash}`, VERSION, this._options.id, this._options.roots.map(root => fastPath.relative(options.rootDir, root)).join(':'), this._options.extensions.join(':'), this._options.platforms.join(':'), this._options.computeSha1.toString(), options.mocksPattern || '', (options.ignorePattern || '').toString(), hasteImplHash, dependencyExtractorHash, this._options.computeDependencies.toString());
  }
  static getCacheFilePath(tmpdir, id, ...extra) {
    const hash = (0, _nodeCrypto().createHash)('sha1').update(extra.join(''));
    return path().join(tmpdir, `${id.replaceAll(/\W/g, '-')}-${hash.digest('hex').slice(0, 32)}`);
  }
  static getModuleMapFromJSON(json) {
    return _ModuleMap.default.fromJSON(json);
  }
  getCacheFilePath() {
    return this._cachePath;
  }
  build() {
    if (!this._buildPromise) {
      this._buildPromise = (async () => {
        const data = await this._buildFileMap();

        // Persist when we don't know if files changed (changedFiles undefined)
        // or when we know a file was changed or deleted.
        let hasteMap;
        if (data.changedFiles === undefined || data.changedFiles.size > 0 || data.removedFiles.size > 0) {
          hasteMap = await this._buildHasteMap(data);
          this._persist(hasteMap);
        } else {
          hasteMap = data.hasteMap;
        }
        const rootDir = this._options.rootDir;
        const hasteFS = new _HasteFS.default({
          files: hasteMap.files,
          rootDir
        });
        const moduleMap = new _ModuleMap.default({
          duplicates: hasteMap.duplicates,
          map: hasteMap.map,
          mocks: hasteMap.mocks,
          rootDir
        });
        const __hasteMapForTest =  false || null;
        await this._watch(hasteMap);
        return {
          __hasteMapForTest,
          hasteFS,
          moduleMap
        };
      })();
    }
    return this._buildPromise;
  }

  /**
   * 1. read data from the cache or create an empty structure.
   */
  read() {
    let hasteMap;
    try {
      hasteMap = (0, _nodeV().deserialize)((0, _gracefulFs().readFileSync)(this._cachePath));
    } catch {
      hasteMap = this._createEmptyMap();
    }
    return hasteMap;
  }
  readModuleMap() {
    const data = this.read();
    return new _ModuleMap.default({
      duplicates: data.duplicates,
      map: data.map,
      mocks: data.mocks,
      rootDir: this._options.rootDir
    });
  }

  /**
   * 2. crawl the file system.
   */
  async _buildFileMap() {
    let hasteMap;
    try {
      const read = this._options.resetCache ? this._createEmptyMap : this.read;
      hasteMap = read.call(this);
    } catch {
      hasteMap = this._createEmptyMap();
    }
    return this._crawl(hasteMap);
  }

  /**
   * 3. parse and extract metadata from changed files.
   */
  _processFile(hasteMap, map, mocks, filePath, workerOptions) {
    const rootDir = this._options.rootDir;
    const setModule = (id, module) => {
      let moduleMap = map.get(id);
      if (!moduleMap) {
        moduleMap = Object.create(null);
        map.set(id, moduleMap);
      }
      const platform = (0, _getPlatformExtension.default)(module[_constants.default.PATH], this._options.platforms) || _constants.default.GENERIC_PLATFORM;
      const existingModule = moduleMap[platform];
      if (existingModule && existingModule[_constants.default.PATH] !== module[_constants.default.PATH]) {
        const method = this._options.throwOnModuleCollision ? 'error' : 'warn';
        this._console[method]([`jest-haste-map: Haste module naming collision: ${id}`, '  The following files share their name; please adjust your hasteImpl:', `    * <rootDir>${path().sep}${existingModule[_constants.default.PATH]}`, `    * <rootDir>${path().sep}${module[_constants.default.PATH]}`, ''].join('\n'));
        if (this._options.throwOnModuleCollision) {
          throw new DuplicateError(existingModule[_constants.default.PATH], module[_constants.default.PATH]);
        }

        // We do NOT want consumers to use a module that is ambiguous.
        delete moduleMap[platform];
        if (Object.keys(moduleMap).length === 1) {
          map.delete(id);
        }
        let dupsByPlatform = hasteMap.duplicates.get(id);
        if (dupsByPlatform == null) {
          dupsByPlatform = new Map();
          hasteMap.duplicates.set(id, dupsByPlatform);
        }
        const dups = new Map([[module[_constants.default.PATH], module[_constants.default.TYPE]], [existingModule[_constants.default.PATH], existingModule[_constants.default.TYPE]]]);
        dupsByPlatform.set(platform, dups);
        return;
      }
      const dupsByPlatform = hasteMap.duplicates.get(id);
      if (dupsByPlatform != null) {
        const dups = dupsByPlatform.get(platform);
        if (dups != null) {
          dups.set(module[_constants.default.PATH], module[_constants.default.TYPE]);
        }
        return;
      }
      moduleMap[platform] = module;
    };
    const relativeFilePath = fastPath.relative(rootDir, filePath);
    const fileMetadata = hasteMap.files.get(relativeFilePath);
    if (!fileMetadata) {
      throw new Error('jest-haste-map: File to process was not found in the haste map.');
    }
    const moduleMetadata = hasteMap.map.get(fileMetadata[_constants.default.ID]);
    const computeSha1 = this._options.computeSha1 && !fileMetadata[_constants.default.SHA1];

    // Callback called when the response from the worker is successful.
    const workerReply = metadata => {
      // `1` for truthy values instead of `true` to save cache space.
      fileMetadata[_constants.default.VISITED] = 1;
      const metadataId = metadata.id;
      const metadataModule = metadata.module;
      if (metadataId && metadataModule) {
        fileMetadata[_constants.default.ID] = metadataId;
        setModule(metadataId, metadataModule);
      }
      fileMetadata[_constants.default.DEPENDENCIES] = metadata.dependencies ? metadata.dependencies.join(_constants.default.DEPENDENCY_DELIM) : '';
      if (computeSha1) {
        fileMetadata[_constants.default.SHA1] = metadata.sha1;
      }
    };

    // Callback called when the response from the worker is an error.
    const workerError = error => {
      if (typeof error !== 'object' || !error.message || !error.stack) {
        error = new Error(error);
        error.stack = ''; // Remove stack for stack-less errors.
      }
      if (!['ENOENT', 'EACCES'].includes(error.code)) {
        throw error;
      }

      // If a file cannot be read we remove it from the file list and
      // ignore the failure silently.
      hasteMap.files.delete(relativeFilePath);
    };

    // If we retain all files in the virtual HasteFS representation, we avoid
    // reading them if they aren't important (node_modules).
    if (this._options.retainAllFiles && filePath.includes(NODE_MODULES)) {
      if (computeSha1) {
        return this._getWorker(workerOptions).getSha1({
          computeDependencies: this._options.computeDependencies,
          computeSha1,
          dependencyExtractor: this._options.dependencyExtractor,
          filePath,
          hasteImplModulePath: this._options.hasteImplModulePath,
          rootDir
        }).then(workerReply, workerError);
      }
      return null;
    }
    if (this._options.mocksPattern && this._options.mocksPattern.test(filePath)) {
      const mockPath = (0, _getMockName.default)(filePath);
      const existingMockPath = mocks.get(mockPath);
      if (existingMockPath) {
        const secondMockPath = fastPath.relative(rootDir, filePath);
        if (existingMockPath !== secondMockPath) {
          const method = this._options.throwOnModuleCollision ? 'error' : 'warn';
          this._console[method]([`jest-haste-map: duplicate manual mock found: ${mockPath}`, '  The following files share their name; please delete one of them:', `    * <rootDir>${path().sep}${existingMockPath}`, `    * <rootDir>${path().sep}${secondMockPath}`, ''].join('\n'));
          if (this._options.throwOnModuleCollision) {
            throw new DuplicateError(existingMockPath, secondMockPath);
          }
        }
      }
      mocks.set(mockPath, relativeFilePath);
    }
    if (fileMetadata[_constants.default.VISITED]) {
      if (!fileMetadata[_constants.default.ID]) {
        return null;
      }
      if (moduleMetadata != null) {
        const platform = (0, _getPlatformExtension.default)(filePath, this._options.platforms) || _constants.default.GENERIC_PLATFORM;
        const module = moduleMetadata[platform];
        if (module == null) {
          return null;
        }
        const moduleId = fileMetadata[_constants.default.ID];
        let modulesByPlatform = map.get(moduleId);
        if (!modulesByPlatform) {
          modulesByPlatform = Object.create(null);
          map.set(moduleId, modulesByPlatform);
        }
        modulesByPlatform[platform] = module;
        return null;
      }
    }
    return this._getWorker(workerOptions).worker({
      computeDependencies: this._options.computeDependencies,
      computeSha1,
      dependencyExtractor: this._options.dependencyExtractor,
      filePath,
      hasteImplModulePath: this._options.hasteImplModulePath,
      rootDir
    }).then(workerReply, workerError);
  }
  _buildHasteMap(data) {
    const {
      removedFiles,
      changedFiles,
      hasteMap
    } = data;

    // If any files were removed or we did not track what files changed, process
    // every file looking for changes. Otherwise, process only changed files.
    let map;
    let mocks;
    let filesToProcess;
    if (changedFiles === undefined || removedFiles.size > 0) {
      map = new Map();
      mocks = new Map();
      filesToProcess = hasteMap.files;
    } else {
      map = hasteMap.map;
      mocks = hasteMap.mocks;
      filesToProcess = changedFiles;
    }
    for (const [relativeFilePath, fileMetadata] of removedFiles) {
      this._recoverDuplicates(hasteMap, relativeFilePath, fileMetadata[_constants.default.ID]);
    }
    const promises = [];
    for (const relativeFilePath of filesToProcess.keys()) {
      if (this._options.skipPackageJson && relativeFilePath.endsWith(PACKAGE_JSON)) {
        continue;
      }
      // SHA-1, if requested, should already be present thanks to the crawler.
      const filePath = fastPath.resolve(this._options.rootDir, relativeFilePath);
      const promise = this._processFile(hasteMap, map, mocks, filePath);
      if (promise) {
        promises.push(promise);
      }
    }
    return Promise.all(promises).then(() => {
      this._cleanup();
      hasteMap.map = map;
      hasteMap.mocks = mocks;
      return hasteMap;
    }, error => {
      this._cleanup();
      throw error;
    });
  }
  _cleanup() {
    const worker = this._worker;
    if (worker && 'end' in worker) {
      worker.end();
    }
    this._worker = null;
  }

  /**
   * 4. serialize the new `HasteMap` in a cache file.
   */
  _persist(hasteMap) {
    (0, _gracefulFs().writeFileSync)(this._cachePath, (0, _nodeV().serialize)(hasteMap));
  }

  /**
   * Creates workers or parses files and extracts metadata in-process.
   */
  _getWorker(options) {
    if (!this._worker) {
      if (options?.forceInBand || this._options.maxWorkers <= 1) {
        this._worker = {
          getSha1: _worker.getSha1,
          worker: _worker.worker
        };
      } else {
        this._worker = new (_jestWorker().Worker)(require.resolve('./worker'), {
          enableWorkerThreads: this._options.workerThreads,
          exposedMethods: ['getSha1', 'worker'],
          forkOptions: {
            serialization: 'json'
          },
          maxRetries: 3,
          numWorkers: this._options.maxWorkers
        });
      }
    }
    return this._worker;
  }
  async _crawl(hasteMap) {
    const options = this._options;
    const ignore = this._ignore.bind(this);
    const crawl = (await this._shouldUseWatchman()) ? _watchman.watchmanCrawl : _node.nodeCrawl;
    const crawlerOptions = {
      computeSha1: options.computeSha1,
      data: hasteMap,
      enableSymlinks: options.enableSymlinks,
      extensions: options.extensions,
      forceNodeFilesystemAPI: options.forceNodeFilesystemAPI,
      ignore,
      rootDir: options.rootDir,
      roots: options.roots
    };
    const retry = retryError => {
      if (crawl === _watchman.watchmanCrawl) {
        this._console.warn('jest-haste-map: Watchman crawl failed. Retrying once with node ' + 'crawler.\n' + "  Usually this happens when watchman isn't running. Create an " + "empty `.watchmanconfig` file in your project's root folder or " + 'initialize a git or hg repository in your project.\n' + `  ${retryError}`);
        return (0, _node.nodeCrawl)(crawlerOptions).catch(error => {
          throw new Error('Crawler retry failed:\n' + `  Original error: ${retryError.message}\n` + `  Retry error: ${error.message}\n`);
        });
      }
      throw retryError;
    };
    try {
      return await crawl(crawlerOptions);
    } catch (error) {
      return retry(error);
    }
  }

  /**
   * Watch mode
   */
  async _watch(hasteMap) {
    if (!this._options.watch) {
      return;
    }

    // In watch mode, we'll only warn about module collisions and we'll retain
    // all files, even changes to node_modules.
    this._options.throwOnModuleCollision = false;
    this._options.retainAllFiles = true;

    // WatchmanWatcher > FSEventsWatcher > sane.NodeWatcher
    const Watcher = (await this._shouldUseWatchman()) ? _WatchmanWatcher.default : _FSEventsWatcher.FSEventsWatcher.isSupported() ? _FSEventsWatcher.FSEventsWatcher : _NodeWatcher.default;
    const extensions = this._options.extensions;
    const ignorePattern = this._options.ignorePattern;
    const rootDir = this._options.rootDir;
    let changeQueue = Promise.resolve();
    let eventsQueue = [];
    // We only need to copy the entire haste map once on every "frame".
    let mustCopy = true;
    const createWatcher = root => {
      const watcher = new Watcher(root, {
        dot: true,
        glob: extensions.map(extension => `**/*.${extension}`),
        ignored: ignorePattern
      });
      return new Promise((resolve, reject) => {
        const rejectTimeout = setTimeout(() => reject(new Error('Failed to start watch mode.')), MAX_WAIT_TIME);
        watcher.once('ready', () => {
          clearTimeout(rejectTimeout);
          watcher.on('all', onChange);
          resolve(watcher);
        });
      });
    };
    const emitChange = () => {
      if (eventsQueue.length > 0) {
        mustCopy = true;
        const changeEvent = {
          eventsQueue,
          hasteFS: new _HasteFS.default({
            files: hasteMap.files,
            rootDir
          }),
          moduleMap: new _ModuleMap.default({
            duplicates: hasteMap.duplicates,
            map: hasteMap.map,
            mocks: hasteMap.mocks,
            rootDir
          })
        };
        this.emit('change', changeEvent);
        eventsQueue = [];
      }
    };
    const onChange = (type, filePath, root, stat) => {
      filePath = path().join(root, (0, _normalizePathSep.default)(filePath));
      if (stat && stat.isDirectory() || this._ignore(filePath) || !extensions.some(extension => filePath.endsWith(extension))) {
        return;
      }
      const relativeFilePath = fastPath.relative(rootDir, filePath);
      const fileMetadata = hasteMap.files.get(relativeFilePath);

      // The file has been accessed, not modified
      if (type === 'change' && fileMetadata && stat && fileMetadata[_constants.default.MTIME] === stat.mtime.getTime()) {
        return;
      }
      changeQueue = changeQueue.then(() => {
        // If we get duplicate events for the same file, ignore them.
        if (eventsQueue.some(event => event.type === type && event.filePath === filePath && (!event.stat && !stat || !!event.stat && !!stat && event.stat.mtime.getTime() === stat.mtime.getTime()))) {
          return null;
        }
        if (mustCopy) {
          mustCopy = false;
          hasteMap = {
            clocks: new Map(hasteMap.clocks),
            duplicates: new Map(hasteMap.duplicates),
            files: new Map(hasteMap.files),
            map: new Map(hasteMap.map),
            mocks: new Map(hasteMap.mocks)
          };
        }
        const add = () => {
          eventsQueue.push({
            filePath,
            stat,
            type
          });
          return null;
        };
        const fileMetadata = hasteMap.files.get(relativeFilePath);

        // If it's not an addition, delete the file and all its metadata
        if (fileMetadata != null) {
          const moduleName = fileMetadata[_constants.default.ID];
          const platform = (0, _getPlatformExtension.default)(filePath, this._options.platforms) || _constants.default.GENERIC_PLATFORM;
          hasteMap.files.delete(relativeFilePath);
          let moduleMap = hasteMap.map.get(moduleName);
          if (moduleMap != null) {
            // We are forced to copy the object because jest-haste-map exposes
            // the map as an immutable entity.
            moduleMap = copy(moduleMap);
            delete moduleMap[platform];
            if (Object.keys(moduleMap).length === 0) {
              hasteMap.map.delete(moduleName);
            } else {
              hasteMap.map.set(moduleName, moduleMap);
            }
          }
          if (this._options.mocksPattern && this._options.mocksPattern.test(filePath)) {
            const mockName = (0, _getMockName.default)(filePath);
            hasteMap.mocks.delete(mockName);
          }
          this._recoverDuplicates(hasteMap, relativeFilePath, moduleName);
        }

        // If the file was added or changed,
        // parse it and update the haste map.
        if (type === 'add' || type === 'change') {
          (0, _jestUtil().invariant)(stat, 'since the file exists or changed, it should have stats');
          const fileMetadata = ['', stat.mtime.getTime(), stat.size, 0, '', null];
          hasteMap.files.set(relativeFilePath, fileMetadata);
          const promise = this._processFile(hasteMap, hasteMap.map, hasteMap.mocks, filePath, {
            forceInBand: true
          });
          // Cleanup
          this._cleanup();
          if (promise) {
            return promise.then(add);
          } else {
            // If a file in node_modules has changed,
            // emit an event regardless.
            add();
          }
        } else {
          add();
        }
        return null;
      }).catch(error => {
        this._console.error(`jest-haste-map: watch error:\n  ${error.stack}\n`);
      });
    };
    this._changeInterval = setInterval(emitChange, CHANGE_INTERVAL);
    return Promise.all(this._options.roots.map(createWatcher)).then(watchers => {
      this._watchers = watchers;
    });
  }

  /**
   * This function should be called when the file under `filePath` is removed
   * or changed. When that happens, we want to figure out if that file was
   * part of a group of files that had the same ID. If it was, we want to
   * remove it from the group. Furthermore, if there is only one file
   * remaining in the group, then we want to restore that single file as the
   * correct resolution for its ID, and cleanup the duplicates index.
   */
  _recoverDuplicates(hasteMap, relativeFilePath, moduleName) {
    let dupsByPlatform = hasteMap.duplicates.get(moduleName);
    if (dupsByPlatform == null) {
      return;
    }
    const platform = (0, _getPlatformExtension.default)(relativeFilePath, this._options.platforms) || _constants.default.GENERIC_PLATFORM;
    let dups = dupsByPlatform.get(platform);
    if (dups == null) {
      return;
    }
    dupsByPlatform = copyMap(dupsByPlatform);
    hasteMap.duplicates.set(moduleName, dupsByPlatform);
    dups = copyMap(dups);
    dupsByPlatform.set(platform, dups);
    dups.delete(relativeFilePath);
    if (dups.size !== 1) {
      return;
    }
    const uniqueModule = dups.entries().next().value;
    if (!uniqueModule) {
      return;
    }
    let dedupMap = hasteMap.map.get(moduleName);
    if (!dedupMap) {
      dedupMap = Object.create(null);
      hasteMap.map.set(moduleName, dedupMap);
    }
    dedupMap[platform] = uniqueModule;
    dupsByPlatform.delete(platform);
    if (dupsByPlatform.size === 0) {
      hasteMap.duplicates.delete(moduleName);
    }
  }
  async end() {
    if (this._changeInterval) {
      clearInterval(this._changeInterval);
    }
    if (this._watchers.length === 0) {
      return;
    }
    await Promise.all(this._watchers.map(watcher => watcher.close()));
    this._watchers = [];
  }

  /**
   * Helpers
   */
  _ignore(filePath) {
    const ignorePattern = this._options.ignorePattern;
    const ignoreMatched = ignorePattern instanceof RegExp ? ignorePattern.test(filePath) : ignorePattern && ignorePattern(filePath);
    return ignoreMatched || !this._options.retainAllFiles && filePath.includes(NODE_MODULES);
  }
  async _shouldUseWatchman() {
    if (!this._options.useWatchman) {
      return false;
    }
    if (!isWatchmanInstalledPromise) {
      isWatchmanInstalledPromise = (0, _isWatchmanInstalled.default)();
    }
    return isWatchmanInstalledPromise;
  }
  _createEmptyMap() {
    return {
      clocks: new Map(),
      duplicates: new Map(),
      files: new Map(),
      map: new Map(),
      mocks: new Map()
    };
  }
  static H = _constants.default;
}
class DuplicateError extends Error {
  mockPath1;
  mockPath2;
  constructor(mockPath1, mockPath2) {
    super('Duplicated files or mocks. Please check the console for more info');
    this.mockPath1 = mockPath1;
    this.mockPath2 = mockPath2;
  }
}
exports.DuplicateError = DuplicateError;
function copy(object) {
  return Object.assign(Object.create(null), object);
}
function copyMap(input) {
  return new Map(input);
}

// Export the smallest API surface required by Jest

const JestHasteMap = HasteMap;
var _default = exports["default"] = JestHasteMap;
})();

module.exports = __webpack_exports__;
/******/ })()
;