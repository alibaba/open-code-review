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

/***/ "./src/ScriptTransformer.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.createScriptTransformer = createScriptTransformer;
exports.createTranspilingRequire = createTranspilingRequire;
function _nodeCrypto() {
  const data = require("node:crypto");
  _nodeCrypto = function () {
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
function _core() {
  const data = require("@babel/core");
  _core = function () {
    return data;
  };
  return data;
}
function _babelPluginIstanbul() {
  const data = _interopRequireDefault(require("babel-plugin-istanbul"));
  _babelPluginIstanbul = function () {
    return data;
  };
  return data;
}
function _convertSourceMap() {
  const data = require("convert-source-map");
  _convertSourceMap = function () {
    return data;
  };
  return data;
}
function _fastJsonStableStringify() {
  const data = _interopRequireDefault(require("fast-json-stable-stringify"));
  _fastJsonStableStringify = function () {
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
function _pirates() {
  const data = require("pirates");
  _pirates = function () {
    return data;
  };
  return data;
}
function _slash() {
  const data = _interopRequireDefault(require("slash"));
  _slash = function () {
    return data;
  };
  return data;
}
function _writeFileAtomic() {
  const data = require("write-file-atomic");
  _writeFileAtomic = function () {
    return data;
  };
  return data;
}
function _jestHasteMap() {
  const data = _interopRequireDefault(require("jest-haste-map"));
  _jestHasteMap = function () {
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
var _enhanceUnexpectedTokenMessage = _interopRequireDefault(__webpack_require__("./src/enhanceUnexpectedTokenMessage.ts"));
var _runtimeErrorsAndWarnings = __webpack_require__("./src/runtimeErrorsAndWarnings.ts");
var _shouldInstrument = _interopRequireDefault(__webpack_require__("./src/shouldInstrument.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// @ts-expect-error: should just be `require.resolve`, but the tests mess that up

// Use `require` to avoid TS rootDir
const {
  version: VERSION
} = __webpack_require__("./package.json");
// This data structure is used to avoid recalculating some data every time that
// we need to transform a file. Since ScriptTransformer is instantiated for each
// file we need to keep this object in the local scope of this module.
const projectCaches = new Map();

// To reset the cache for specific changesets (rather than package version).
const CACHE_VERSION = '1';
async function waitForPromiseWithCleanup(promise, cleanup) {
  try {
    await promise;
  } finally {
    cleanup();
  }
}

// type predicate
function isTransformerFactory(t) {
  return typeof t.createTransformer === 'function';
}
class ScriptTransformer {
  _cache;
  _transformCache = new Map();
  _transformsAreLoaded = false;
  constructor(_config, _cacheFS) {
    this._config = _config;
    this._cacheFS = _cacheFS;
    const configString = (0, _fastJsonStableStringify().default)(this._config);
    let projectCache = projectCaches.get(configString);
    if (!projectCache) {
      projectCache = {
        configString,
        ignorePatternsRegExp: calcIgnorePatternRegExp(this._config),
        transformRegExp: calcTransformRegExp(this._config),
        transformedFiles: new Map()
      };
      projectCaches.set(configString, projectCache);
    }
    this._cache = projectCache;
  }
  _buildCacheKeyFromFileInfo(fileData, filename, transformOptions, transformerCacheKey) {
    if (transformerCacheKey != null) {
      return (0, _nodeCrypto().createHash)('sha1').update(transformerCacheKey).update(CACHE_VERSION).digest('hex').slice(0, 32);
    }
    return (0, _nodeCrypto().createHash)('sha1').update(fileData).update(transformOptions.configString).update(transformOptions.instrument ? 'instrument' : '').update(filename).update(CACHE_VERSION).digest('hex').slice(0, 32);
  }
  _buildTransformCacheKey(pattern, filepath) {
    return pattern + filepath;
  }
  _getCacheKey(fileData, filename, options) {
    const configString = this._cache.configString;
    const {
      transformer,
      transformerConfig = {}
    } = this._getTransformer(filename) ?? {};
    let transformerCacheKey = undefined;
    const transformOptions = {
      ...options,
      cacheFS: this._cacheFS,
      config: this._config,
      configString,
      transformerConfig
    };
    if (typeof transformer?.getCacheKey === 'function') {
      transformerCacheKey = transformer.getCacheKey(fileData, filename, transformOptions);
    }
    return this._buildCacheKeyFromFileInfo(fileData, filename, transformOptions, transformerCacheKey);
  }
  async _getCacheKeyAsync(fileData, filename, options) {
    const configString = this._cache.configString;
    const {
      transformer,
      transformerConfig = {}
    } = this._getTransformer(filename) ?? {};
    let transformerCacheKey = undefined;
    const transformOptions = {
      ...options,
      cacheFS: this._cacheFS,
      config: this._config,
      configString,
      transformerConfig
    };
    if (transformer) {
      const getCacheKey = transformer.getCacheKeyAsync ?? transformer.getCacheKey;
      if (typeof getCacheKey === 'function') {
        transformerCacheKey = await getCacheKey(fileData, filename, transformOptions);
      }
    }
    return this._buildCacheKeyFromFileInfo(fileData, filename, transformOptions, transformerCacheKey);
  }
  _createCachedFilename(filename, cacheKey) {
    const HasteMapClass = _jestHasteMap().default.getStatic(this._config);
    const baseCacheDir = HasteMapClass.getCacheFilePath(this._config.cacheDirectory, `jest-transform-cache-${this._config.id}`, VERSION);
    // Create sub folders based on the cacheKey to avoid creating one
    // directory with many files.
    const cacheDir = path().join(baseCacheDir, cacheKey[0] + cacheKey[1]);
    const cacheFilenamePrefix = path().basename(filename, path().extname(filename)).replaceAll(/\W/g, '');
    return (0, _slash().default)(path().join(cacheDir, `${cacheFilenamePrefix}_${cacheKey}`));
  }
  _getFileCachePath(filename, content, options) {
    const cacheKey = this._getCacheKey(content, filename, options);
    return this._createCachedFilename(filename, cacheKey);
  }
  async _getFileCachePathAsync(filename, content, options) {
    const cacheKey = await this._getCacheKeyAsync(content, filename, options);
    return this._createCachedFilename(filename, cacheKey);
  }
  _getTransformPatternAndPath(filename) {
    const transformEntry = this._cache.transformRegExp;
    if (transformEntry == null) {
      return undefined;
    }
    for (const item of transformEntry) {
      const [transformRegExp, transformPath] = item;
      if (transformRegExp.test(filename)) {
        return [transformRegExp.source, transformPath];
      }
    }
    return undefined;
  }
  _getTransformPath(filename) {
    const transformInfo = this._getTransformPatternAndPath(filename);
    if (!Array.isArray(transformInfo)) {
      return undefined;
    }
    return transformInfo[1];
  }
  async loadTransformers() {
    await Promise.all(this._config.transform.map(async ([transformPattern, transformPath, transformerConfig], i) => {
      let transformer = await (0, _jestUtil().requireOrImportModule)(transformPath);
      if (transformer == null) {
        throw new Error((0, _runtimeErrorsAndWarnings.makeInvalidTransformerError)(transformPath));
      }
      if (isTransformerFactory(transformer)) {
        transformer = await transformer.createTransformer(transformerConfig);
      }
      if (typeof transformer.process !== 'function' && typeof transformer.processAsync !== 'function') {
        throw new TypeError((0, _runtimeErrorsAndWarnings.makeInvalidTransformerError)(transformPath));
      }
      const res = {
        transformer,
        transformerConfig
      };
      const transformCacheKey = this._buildTransformCacheKey(this._cache.transformRegExp?.[i]?.[0].source ?? new RegExp(transformPattern).source, transformPath);
      this._transformCache.set(transformCacheKey, res);
    }));
    this._transformsAreLoaded = true;
  }
  _getTransformer(filename) {
    if (!this._transformsAreLoaded) {
      throw new Error('Jest: Transformers have not been loaded yet - make sure to run `loadTransformers` and wait for it to complete before starting to transform files');
    }
    if (this._config.transform.length === 0) {
      return null;
    }
    const transformPatternAndPath = this._getTransformPatternAndPath(filename);
    if (!Array.isArray(transformPatternAndPath)) {
      return null;
    }
    const [transformPattern, transformPath] = transformPatternAndPath;
    const transformCacheKey = this._buildTransformCacheKey(transformPattern, transformPath);
    const transformer = this._transformCache.get(transformCacheKey);
    if (transformer !== undefined) {
      return transformer;
    }
    throw new Error(`Jest was unable to load the transformer defined for ${filename}. This is a bug in Jest, please open up an issue`);
  }
  _instrumentFile(filename, input, canMapToInput, options) {
    const inputCode = typeof input === 'string' ? input : input.code;
    const inputMap = typeof input === 'string' ? null : input.map;
    const result = (0, _core().transformSync)(inputCode, {
      auxiliaryCommentBefore: ' istanbul ignore next ',
      babelrc: false,
      caller: {
        name: '@jest/transform',
        supportsDynamicImport: options.supportsDynamicImport,
        supportsExportNamespaceFrom: options.supportsExportNamespaceFrom,
        supportsStaticESM: options.supportsStaticESM,
        supportsTopLevelAwait: options.supportsTopLevelAwait
      },
      configFile: false,
      filename,
      plugins: [[_babelPluginIstanbul().default, {
        compact: false,
        // files outside `cwd` will not be instrumented
        cwd: this._config.rootDir,
        exclude: [],
        extension: false,
        inputSourceMap: inputMap,
        useInlineSourceMaps: false
      }]],
      sourceMaps: canMapToInput ? 'both' : false
    });
    if (result?.code != null) {
      return result;
    }
    return input;
  }
  _buildTransformResult(filename, cacheFilePath, content, transformer, shouldCallTransform, options, processed, sourceMapPath) {
    let transformed = {
      code: content,
      map: null
    };
    if (transformer && shouldCallTransform) {
      if (processed != null && typeof processed.code === 'string') {
        transformed = processed;
      } else {
        const transformPath = this._getTransformPath(filename);
        (0, _jestUtil().invariant)(transformPath);
        throw new Error((0, _runtimeErrorsAndWarnings.makeInvalidReturnValueError)(transformPath));
      }
    }
    if (transformed.map == null || transformed.map === '') {
      try {
        //Could be a potential freeze here.
        //See: https://github.com/jestjs/jest/pull/5177#discussion_r158883570
        const inlineSourceMap = (0, _convertSourceMap().fromSource)(transformed.code);
        if (inlineSourceMap) {
          transformed.map = inlineSourceMap.toObject();
        }
      } catch {
        const transformPath = this._getTransformPath(filename);
        (0, _jestUtil().invariant)(transformPath);
        console.warn((0, _runtimeErrorsAndWarnings.makeInvalidSourceMapWarning)(filename, transformPath));
      }
    }

    // That means that the transform has a custom instrumentation
    // logic and will handle it based on `config.collectCoverage` option
    const transformWillInstrument = shouldCallTransform && transformer?.canInstrument;

    // Apply instrumentation to the code if necessary, keeping the instrumented code and new map
    let map = transformed.map;
    let code;
    if (transformWillInstrument !== true && options.instrument) {
      /**
       * We can map the original source code to the instrumented code ONLY if
       * - the process of transforming the code produced a source map e.g. ts-jest
       * - we did not transform the source code
       *
       * Otherwise we cannot make any statements about how the instrumented code corresponds to the original code,
       * and we should NOT emit any source maps
       *
       */
      const shouldEmitSourceMaps = transformer != null && map != null || transformer == null;
      const instrumented = this._instrumentFile(filename, transformed, shouldEmitSourceMaps, options);
      code = typeof instrumented === 'string' ? instrumented : instrumented.code;
      map = typeof instrumented === 'string' ? null : instrumented.map;
    } else {
      code = transformed.code;
    }
    if (map == null) {
      sourceMapPath = null;
    } else {
      const sourceMapContent = typeof map === 'string' ? map : JSON.stringify(map);
      (0, _jestUtil().invariant)(sourceMapPath, 'We should always have default sourceMapPath');
      writeCacheFile(sourceMapPath, sourceMapContent);
    }
    writeCodeCacheFile(cacheFilePath, code);
    return {
      code,
      originalCode: content,
      sourceMapPath
    };
  }
  transformSource(filepath, content, options) {
    const filename = (0, _jestUtil().tryRealpath)(filepath);
    const {
      transformer,
      transformerConfig = {}
    } = this._getTransformer(filename) ?? {};
    const cacheFilePath = this._getFileCachePath(filename, content, options);
    const sourceMapPath = `${cacheFilePath}.map`;
    // Ignore cache if `config.cache` is set (--no-cache)
    const code = this._config.cache ? readCodeCacheFile(cacheFilePath) : null;
    if (code != null) {
      // This is broken: we return the code, and a path for the source map
      // directly from the cache. But, nothing ensures the source map actually
      // matches that source code. They could have gotten out-of-sync in case
      // two separate processes write concurrently to the same cache files.
      return {
        code,
        originalCode: content,
        sourceMapPath
      };
    }
    let processed = null;
    let shouldCallTransform = false;
    if (transformer && this.shouldTransform(filename)) {
      shouldCallTransform = true;
      assertSyncTransformer(transformer, this._getTransformPath(filename));
      processed = transformer.process(content, filename, {
        ...options,
        cacheFS: this._cacheFS,
        config: this._config,
        configString: this._cache.configString,
        transformerConfig
      });
    }
    (0, _jestUtil().createDirectory)(path().dirname(cacheFilePath));
    return this._buildTransformResult(filename, cacheFilePath, content, transformer, shouldCallTransform, options, processed, sourceMapPath);
  }
  async transformSourceAsync(filepath, content, options) {
    const filename = (0, _jestUtil().tryRealpath)(filepath);
    const {
      transformer,
      transformerConfig = {}
    } = this._getTransformer(filename) ?? {};
    const cacheFilePath = await this._getFileCachePathAsync(filename, content, options);
    const sourceMapPath = `${cacheFilePath}.map`;
    // Ignore cache if `config.cache` is set (--no-cache)
    const code = this._config.cache ? readCodeCacheFile(cacheFilePath) : null;
    if (code != null) {
      // This is broken: we return the code, and a path for the source map
      // directly from the cache. But, nothing ensures the source map actually
      // matches that source code. They could have gotten out-of-sync in case
      // two separate processes write concurrently to the same cache files.
      return {
        code,
        originalCode: content,
        sourceMapPath
      };
    }
    let processed = null;
    let shouldCallTransform = false;
    if (transformer && this.shouldTransform(filename)) {
      shouldCallTransform = true;
      const process = transformer.processAsync ?? transformer.process;

      // This is probably dead code since `_getTransformerAsync` already asserts this
      (0, _jestUtil().invariant)(typeof process === 'function', 'A transformer must always export either a `process` or `processAsync`');
      processed = await process(content, filename, {
        ...options,
        cacheFS: this._cacheFS,
        config: this._config,
        configString: this._cache.configString,
        transformerConfig
      });
    }
    (0, _jestUtil().createDirectory)(path().dirname(cacheFilePath));
    return this._buildTransformResult(filename, cacheFilePath, content, transformer, shouldCallTransform, options, processed, sourceMapPath);
  }
  async _transformAndBuildScriptAsync(filename, options, transformOptions, fileSource) {
    const {
      isInternalModule
    } = options;
    let fileContent = fileSource ?? this._cacheFS.get(filename);
    if (fileContent == null) {
      fileContent = fs().readFileSync(filename, 'utf8');
      this._cacheFS.set(filename, fileContent);
    }
    const content = stripShebang(fileContent);
    let code = content;
    let sourceMapPath = null;
    const willTransform = isInternalModule !== true && (transformOptions.instrument || this.shouldTransform(filename));
    try {
      if (willTransform) {
        const transformedSource = await this.transformSourceAsync(filename, content, transformOptions);
        code = transformedSource.code;
        sourceMapPath = transformedSource.sourceMapPath;
      }
      return {
        code,
        originalCode: content,
        sourceMapPath
      };
    } catch (error) {
      if (!(error instanceof Error)) {
        throw error;
      }
      throw (0, _enhanceUnexpectedTokenMessage.default)(error);
    }
  }
  _transformAndBuildScript(filename, options, transformOptions, fileSource) {
    const {
      isInternalModule
    } = options;
    let fileContent = fileSource ?? this._cacheFS.get(filename);
    if (fileContent == null) {
      fileContent = fs().readFileSync(filename, 'utf8');
      this._cacheFS.set(filename, fileContent);
    }
    const content = stripShebang(fileContent);
    let code = content;
    let sourceMapPath = null;
    const willTransform = isInternalModule !== true && (transformOptions.instrument || this.shouldTransform(filename));
    try {
      if (willTransform) {
        const transformedSource = this.transformSource(filename, content, transformOptions);
        code = transformedSource.code;
        sourceMapPath = transformedSource.sourceMapPath;
      }
      return {
        code,
        originalCode: content,
        sourceMapPath
      };
    } catch (error) {
      if (!(error instanceof Error)) {
        throw error;
      }
      throw (0, _enhanceUnexpectedTokenMessage.default)(error);
    }
  }
  async transformAsync(filename, options, fileSource) {
    const instrument = options.coverageProvider === 'babel' && (0, _shouldInstrument.default)(filename, options, this._config);
    const scriptCacheKey = getScriptCacheKey(filename, instrument);
    let result = this._cache.transformedFiles.get(scriptCacheKey);
    if (result) {
      return result;
    }
    result = await this._transformAndBuildScriptAsync(filename, options, {
      ...options,
      instrument
    }, fileSource);
    if (scriptCacheKey) {
      this._cache.transformedFiles.set(scriptCacheKey, result);
    }
    return result;
  }
  transform(filename, options, fileSource) {
    const instrument = options.coverageProvider === 'babel' && (0, _shouldInstrument.default)(filename, options, this._config);
    const scriptCacheKey = getScriptCacheKey(filename, instrument);
    let result = this._cache.transformedFiles.get(scriptCacheKey);
    if (result) {
      return result;
    }
    result = this._transformAndBuildScript(filename, options, {
      ...options,
      instrument
    }, fileSource);
    if (scriptCacheKey) {
      this._cache.transformedFiles.set(scriptCacheKey, result);
    }
    return result;
  }
  transformJson(filename, options, fileSource) {
    const {
      isInternalModule
    } = options;
    const willTransform = isInternalModule !== true && this.shouldTransform(filename);
    if (willTransform) {
      const {
        code: transformedJsonSource
      } = this.transformSource(filename, fileSource, {
        ...options,
        instrument: false
      });
      return transformedJsonSource;
    }
    return fileSource;
  }
  async requireAndTranspileModule(moduleName, callback, options) {
    options = {
      applyInteropRequireDefault: true,
      instrument: false,
      supportsDynamicImport: false,
      supportsExportNamespaceFrom: false,
      supportsStaticESM: false,
      supportsTopLevelAwait: false,
      ...options
    };
    let transforming = false;
    const {
      applyInteropRequireDefault,
      ...transformOptions
    } = options;
    const revertHook = (0, _pirates().addHook)((code, filename) => {
      try {
        transforming = true;
        return this.transformSource(filename, code, transformOptions).code || code;
      } finally {
        transforming = false;
      }
    }, {
      // Exclude `mjs` extension when addHook because pirates don't support hijack es module
      exts: this._config.moduleFileExtensions.filter(ext => ext !== 'mjs').map(ext => `.${ext}`),
      ignoreNodeModules: false,
      matcher: filename => {
        if (transforming) {
          // Don't transform any dependency required by the transformer itself
          return false;
        }
        return this.shouldTransform(filename);
      }
    });
    try {
      const module = await (0, _jestUtil().requireOrImportModule)(moduleName, applyInteropRequireDefault);
      if (!callback) {
        revertHook();
        return module;
      }
      const cbResult = callback(module);
      if ((0, _jestUtil().isPromise)(cbResult)) {
        return await waitForPromiseWithCleanup(cbResult, revertHook).then(() => module);
      }
      return module;
    } finally {
      revertHook();
    }
  }
  shouldTransform(filename) {
    const ignoreRegexp = this._cache.ignorePatternsRegExp;
    const isIgnored = ignoreRegexp ? ignoreRegexp.test(filename) : false;
    return this._config.transform.length > 0 && !isIgnored;
  }
  canTransformSync(filename) {
    if (!this.shouldTransform(filename)) {
      return true;
    }
    const transformerEntry = this._getTransformer(filename);
    if (transformerEntry == null) {
      return true;
    }
    return typeof transformerEntry.transformer.process === 'function';
  }
}
async function createTranspilingRequire(config) {
  const transformer = await createScriptTransformer(config);
  return async function requireAndTranspileModule(resolverPath, applyInteropRequireDefault = false) {
    const transpiledModule = await transformer.requireAndTranspileModule(resolverPath,
    // eslint-disable-next-line @typescript-eslint/no-empty-function
    () => {}, {
      applyInteropRequireDefault,
      instrument: false,
      supportsDynamicImport: false,
      // this might be true, depending on node version.
      supportsExportNamespaceFrom: false,
      supportsStaticESM: false,
      supportsTopLevelAwait: false
    });
    return transpiledModule;
  };
}
const removeFile = path => {
  try {
    fs().unlinkSync(path);
  } catch {}
};
const stripShebang = content => {
  // If the file data starts with a shebang remove it. Leaves the empty line
  // to keep stack trace line numbers correct.
  if (content.startsWith('#!')) {
    return content.replace(/^#!.*/, '');
  } else {
    return content;
  }
};

/**
 * This is like `writeCacheFile` but with an additional sanity checksum. We
 * cannot use the same technique for source maps because we expose source map
 * cache file paths directly to callsites, with the expectation they can read
 * it right away. This is not a great system, because source map cache file
 * could get corrupted, out-of-sync, etc.
 */
function writeCodeCacheFile(cachePath, code) {
  const checksum = (0, _nodeCrypto().createHash)('sha1').update(code).digest('hex').slice(0, 32);
  writeCacheFile(cachePath, `${checksum}\n${code}`);
}

/**
 * Read counterpart of `writeCodeCacheFile`. We verify that the content of the
 * file matches the checksum, in case some kind of corruption happened. This
 * could happen if an older version of `jest-runtime` writes non-atomically to
 * the same cache, for example.
 */
function readCodeCacheFile(cachePath) {
  const content = readCacheFile(cachePath);
  if (content == null) {
    return null;
  }
  const code = content.slice(33);
  const checksum = (0, _nodeCrypto().createHash)('sha1').update(code).digest('hex').slice(0, 32);
  if (checksum === content.slice(0, 32)) {
    return code;
  }
  return null;
}

/**
 * Writing to the cache atomically relies on 'rename' being atomic on most
 * file systems. Doing atomic write reduces the risk of corruption by avoiding
 * two processes to write to the same file at the same time. It also reduces
 * the risk of reading a file that's being overwritten at the same time.
 */
const writeCacheFile = (cachePath, fileData) => {
  try {
    (0, _writeFileAtomic().sync)(cachePath, fileData, {
      encoding: 'utf8',
      fsync: false
    });
  } catch (error) {
    if (!(error instanceof Error)) {
      throw error;
    }
    if (cacheWriteErrorSafeToIgnore(error)) {
      return;
    }
    error.message = `jest: failed to cache transform results in: ${cachePath}\nFailure message: ${error.message}`;
    removeFile(cachePath);
    throw error;
  }
};

/**
 * On Windows, renames are not atomic, leading to EPERM exceptions when two
 * processes attempt to rename to the same target file at the same time.
 * If the target file exists we can be reasonably sure another process has
 * legitimately won a cache write race and ignore the error.
 * If the target does not exist we do not know if it is because it is still
 * being written by another process or is being overwritten by another process.
 */
const cacheWriteErrorSafeToIgnore = e => process.platform === 'win32' && e.code === 'EPERM';
const readCacheFile = cachePath => {
  if (!fs().existsSync(cachePath)) {
    return null;
  }
  let fileData;
  try {
    fileData = fs().readFileSync(cachePath, 'utf8');
  } catch (error) {
    if (!(error instanceof Error)) {
      throw error;
    }
    // on windows write-file-atomic is not atomic which can
    // result in this error
    if (error.code === 'ENOENT' && process.platform === 'win32') {
      return null;
    }
    error.message = `jest: failed to read cache file: ${cachePath}\nFailure message: ${error.message}`;
    removeFile(cachePath);
    throw error;
  }
  if (fileData == null) {
    // We must have somehow created the file but failed to write to it,
    // let's delete it and retry.
    removeFile(cachePath);
  }
  return fileData;
};
const getScriptCacheKey = (filename, instrument) => {
  const mtime = fs().statSync(filename).mtime;
  return `${filename}_${mtime.getTime()}${instrument ? '_instrumented' : ''}`;
};
const calcIgnorePatternRegExp = config => {
  if (config.transformIgnorePatterns == null || config.transformIgnorePatterns.length === 0) {
    return undefined;
  }
  return new RegExp(config.transformIgnorePatterns.join('|'));
};
const calcTransformRegExp = config => {
  if (config.transform.length === 0) {
    return undefined;
  }
  const transformRegexp = [];
  for (const item of config.transform) {
    transformRegexp.push([new RegExp(item[0]), item[1], item[2]]);
  }
  return transformRegexp;
};
function assertSyncTransformer(transformer, name) {
  (0, _jestUtil().invariant)(name);
  (0, _jestUtil().invariant)(typeof transformer.process === 'function', (0, _runtimeErrorsAndWarnings.makeInvalidSyncTransformerError)(name));
}
async function createScriptTransformer(config, cacheFS = new Map()) {
  const transformer = new ScriptTransformer(config, cacheFS);
  await transformer.loadTransformers();
  return transformer;
}

/***/ },

/***/ "./src/enhanceUnexpectedTokenMessage.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = handlePotentialSyntaxError;
exports.enhanceUnexpectedTokenMessage = enhanceUnexpectedTokenMessage;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const DOT = ' \u2022 ';
function handlePotentialSyntaxError(e) {
  if (e.codeFrame != null) {
    e.stack = `${e.message}\n${e.codeFrame}`;
  }
  if (
  // `instanceof` might come from the wrong context
  e.name === 'SyntaxError' && !e.message.includes(' expected')) {
    throw enhanceUnexpectedTokenMessage(e);
  }
  return e;
}
function enhanceUnexpectedTokenMessage(e) {
  e.stack = `${_chalk().default.bold.red('Jest encountered an unexpected token')}

Jest failed to parse a file. This happens e.g. when your code or its dependencies use non-standard JavaScript syntax, or when Jest is not configured to support such syntax.

Out of the box Jest supports Babel, which will be used to transform your files into valid JS based on your Babel configuration.

By default "node_modules" folder is ignored by transformers.

Here's what you can do:
${DOT}If you are trying to use ECMAScript Modules, see ${_chalk().default.underline('https://jestjs.io/docs/ecmascript-modules')} for how to enable it.
${DOT}If you are trying to use TypeScript, see ${_chalk().default.underline('https://jestjs.io/docs/getting-started#using-typescript')}
${DOT}To have some of your "node_modules" files transformed, you can specify a custom ${_chalk().default.bold('"transformIgnorePatterns"')} in your config.
${DOT}If you need a custom transformation, specify a ${_chalk().default.bold('"transform"')} option in your config.
${DOT}If you simply want to mock your non-JS modules (e.g. binary assets) you can stub them out with the ${_chalk().default.bold('"moduleNameMapper"')} config option.

You'll find more details and examples of these config options in the docs:
${_chalk().default.cyan('https://jestjs.io/docs/configuration')}
For information about custom transformations, see:
${_chalk().default.cyan('https://jestjs.io/docs/code-transformation')}

${_chalk().default.bold.red('Details:')}

${e.stack ?? ''}`.trimEnd();
  return e;
}

/***/ },

/***/ "./src/runtimeErrorsAndWarnings.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.makeInvalidTransformerError = exports.makeInvalidSyncTransformerError = exports.makeInvalidSourceMapWarning = exports.makeInvalidReturnValueError = void 0;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _slash() {
  const data = _interopRequireDefault(require("slash"));
  _slash = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const BULLET = '\u25CF ';
const DOCUMENTATION_NOTE = `  ${_chalk().default.bold('Code Transformation Documentation:')}
  https://jestjs.io/docs/code-transformation
`;
const UPGRADE_NOTE = `  ${_chalk().default.bold('This error may be caused by a breaking change in Jest 28:')}
  https://jest-archive-august-2023.netlify.app/docs/28.x/upgrading-to-jest28#transformer
`;
const makeInvalidReturnValueError = transformPath => _chalk().default.red([_chalk().default.bold(`${BULLET}Invalid return value:`), '  `process()` or/and `processAsync()` method of code transformer found at ', `  "${(0, _slash().default)(transformPath)}" `, '  should return an object or a Promise resolving to an object. The object ', '  must have `code` property with a string of processed code.', ''].join('\n') + UPGRADE_NOTE + DOCUMENTATION_NOTE);
exports.makeInvalidReturnValueError = makeInvalidReturnValueError;
const makeInvalidSourceMapWarning = (filename, transformPath) => _chalk().default.yellow([_chalk().default.bold(`${BULLET}Invalid source map:`), `  The source map for "${(0, _slash().default)(filename)}" returned by "${(0, _slash().default)(transformPath)}" is invalid.`, '  Proceeding without source mapping for that file.'].join('\n'));
exports.makeInvalidSourceMapWarning = makeInvalidSourceMapWarning;
const makeInvalidSyncTransformerError = transformPath => _chalk().default.red([_chalk().default.bold(`${BULLET}Invalid synchronous transformer module:`), `  "${(0, _slash().default)(transformPath)}" specified in the "transform" object of Jest configuration`, '  must export a `process` function.', ''].join('\n') + DOCUMENTATION_NOTE);
exports.makeInvalidSyncTransformerError = makeInvalidSyncTransformerError;
const makeInvalidTransformerError = transformPath => _chalk().default.red([_chalk().default.bold(`${BULLET}Invalid transformer module:`), `  "${(0, _slash().default)(transformPath)}" specified in the "transform" object of Jest configuration`, '  must export a `process` or `processAsync` or `createTransformer` function.', ''].join('\n') + DOCUMENTATION_NOTE);
exports.makeInvalidTransformerError = makeInvalidTransformerError;

/***/ },

/***/ "./src/shouldInstrument.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = shouldInstrument;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const MOCKS_PATTERN = new RegExp((0, _jestRegexUtil().escapePathForRegex)(`${path().sep}__mocks__${path().sep}`));
const cachedRegexes = new Map();
const getRegex = regexStr => {
  if (!cachedRegexes.has(regexStr)) {
    cachedRegexes.set(regexStr, new RegExp(regexStr));
  }
  const regex = cachedRegexes.get(regexStr);

  // prevent stateful regexes from breaking, just in case
  regex.lastIndex = 0;
  return regex;
};
function shouldInstrument(filename, options, config, loadedFilenames) {
  if (!options.collectCoverage) {
    return false;
  }
  if (config.forceCoverageMatch.length > 0 && (0, _jestUtil().globsToMatcher)(config.forceCoverageMatch)(filename)) {
    return true;
  }
  if (!config.testPathIgnorePatterns.some(pattern => getRegex(pattern).test(filename))) {
    if (config.testRegex.some(regex => new RegExp(regex).test(filename))) {
      return false;
    }
    if ((0, _jestUtil().globsToMatcher)(config.testMatch)((0, _jestUtil().replacePathSepForGlob)(filename))) {
      return false;
    }
  }
  if (options.collectCoverageFrom.length === 0 && loadedFilenames != null && !loadedFilenames.includes(filename)) {
    return false;
  }
  if (
  // still cover if `only` is specified
  options.collectCoverageFrom.length > 0 && !(0, _jestUtil().globsToMatcher)(options.collectCoverageFrom)((0, _jestUtil().replacePathSepForGlob)(path().relative(options.globalRootDir ?? config.rootDir, filename)))) {
    return false;
  }
  if (config.coveragePathIgnorePatterns.some(pattern => new RegExp(pattern).test(filename))) {
    return false;
  }
  if (config.globalSetup === filename) {
    return false;
  }
  if (config.globalTeardown === filename) {
    return false;
  }
  if (config.setupFiles.includes(filename)) {
    return false;
  }
  if (config.setupFilesAfterEnv.includes(filename)) {
    return false;
  }
  if (MOCKS_PATTERN.test(filename)) {
    return false;
  }
  if (options.changedFiles && !options.changedFiles.has(filename)) {
    if (!options.sourcesRelatedToTestsInChangedFiles) {
      return false;
    }
    if (!options.sourcesRelatedToTestsInChangedFiles.has(filename)) {
      return false;
    }
  }
  if (filename.endsWith('.json')) {
    return false;
  }
  return true;
}

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
var __webpack_exports__ = {};
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
Object.defineProperty(exports, "createScriptTransformer", ({
  enumerable: true,
  get: function () {
    return _ScriptTransformer.createScriptTransformer;
  }
}));
Object.defineProperty(exports, "createTranspilingRequire", ({
  enumerable: true,
  get: function () {
    return _ScriptTransformer.createTranspilingRequire;
  }
}));
Object.defineProperty(exports, "handlePotentialSyntaxError", ({
  enumerable: true,
  get: function () {
    return _enhanceUnexpectedTokenMessage.default;
  }
}));
Object.defineProperty(exports, "shouldInstrument", ({
  enumerable: true,
  get: function () {
    return _shouldInstrument.default;
  }
}));
var _ScriptTransformer = __webpack_require__("./src/ScriptTransformer.ts");
var _shouldInstrument = _interopRequireDefault(__webpack_require__("./src/shouldInstrument.ts"));
var _enhanceUnexpectedTokenMessage = _interopRequireDefault(__webpack_require__("./src/enhanceUnexpectedTokenMessage.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
})();

module.exports = __webpack_exports__;
/******/ })()
;