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

/***/ "./src/helpers.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.findSiblingsWithFileExtension = exports.decodePossibleOutsideJestVmPath = exports.createOutsideJestVmPath = void 0;
exports.noop = noop;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _glob() {
  const data = require("glob");
  _glob = function () {
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
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const OUTSIDE_JEST_VM_PROTOCOL = 'jest-main:';
// String manipulation is easier here, fileURLToPath is only in newer Nodes,
// plus setting non-standard protocols on URL objects is difficult.
const createOutsideJestVmPath = path => `${OUTSIDE_JEST_VM_PROTOCOL}//${encodeURIComponent(path)}`;
exports.createOutsideJestVmPath = createOutsideJestVmPath;
const decodePossibleOutsideJestVmPath = outsideJestVmPath => {
  if (outsideJestVmPath.startsWith(OUTSIDE_JEST_VM_PROTOCOL)) {
    return decodeURIComponent(outsideJestVmPath.replace(new RegExp(`^${OUTSIDE_JEST_VM_PROTOCOL}//`), ''));
  }
  return undefined;
};
exports.decodePossibleOutsideJestVmPath = decodePossibleOutsideJestVmPath;
const findSiblingsWithFileExtension = (moduleFileExtensions, from, moduleName) => {
  if (!path().isAbsolute(moduleName) && path().extname(moduleName) === '') {
    const dirname = path().dirname(from);
    const pathToModule = path().resolve(dirname, moduleName);
    try {
      const slashedDirname = (0, _slash().default)(dirname);
      const matches = _glob().glob.sync(`${pathToModule}.*`, {
        windowsPathsNoEscape: true
      }).map(match => {
        const slashedMap = (0, _slash().default)(match);
        const relativePath = path().posix.relative(slashedDirname, slashedMap);
        const slashedPath = path().posix.dirname(slashedMap) === slashedDirname ? `./${relativePath}` : relativePath;
        return `\t'${slashedPath}'`;
      }).join('\n');
      if (matches) {
        const foundMessage = `\n\nHowever, Jest was able to find:\n${matches}`;
        const mappedModuleFileExtensions = moduleFileExtensions.map(ext => `'${ext}'`).join(', ');
        return `${foundMessage}\n\nYou might want to include a file extension in your import, or update your 'moduleFileExtensions', which is currently ` + `[${mappedModuleFileExtensions}].\n\nSee https://jestjs.io/docs/configuration#modulefileextensions-arraystring`;
      }
    } catch {}
  }
  return '';
};
exports.findSiblingsWithFileExtension = findSiblingsWithFileExtension;
function noop() {
  // empty
}

/***/ },

/***/ "./src/internals/CjsExportsCache.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.CjsExportsCache = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _cjsModuleLexer() {
  const data = require("cjs-module-lexer");
  _cjsModuleLexer = function () {
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

// Computes (and caches) the named exports of a CJS module by static analysis
// with cjs-module-lexer, recursively walking `module.exports = require(...)`
// re-exports. Native (`.node`) addons and core-module re-exports can't be
// statically analysed, so they are loaded via the injected callbacks and the
// real export keys are read off the resulting object.
class CjsExportsCache {
  cache = new Map();
  resolution;
  fileCache;
  transformCache;
  loadNativeAddon;
  loadCoreReexport;
  constructor(options) {
    this.resolution = options.resolution;
    this.fileCache = options.fileCache;
    this.transformCache = options.transformCache;
    this.loadNativeAddon = options.loadNativeAddon;
    this.loadCoreReexport = options.loadCoreReexport;
  }

  // `from` is the module asking for the exports - propagated to the load
  // callbacks so user mocks (`jest.mock('./addon.node', factory)`) dispatch
  // against the real importer rather than an empty placeholder. The cache is
  // keyed by `modulePath` only (export keys don't depend on the importer);
  // `from` matters only for the cache-miss load.
  getExportsOf(from, modulePath) {
    const cached = this.cache.get(modulePath);
    if (cached) return cached;
    if (path().extname(modulePath) === '.node') {
      const nativeModule = this.loadNativeAddon(from, modulePath);
      const namedExports = new Set(Object.keys(nativeModule));
      this.cache.set(modulePath, namedExports);
      return namedExports;
    }
    const transformedCode = this.transformCache.getCachedSource(modulePath) ?? this.fileCache.readFile(modulePath);
    const {
      exports,
      reexports
    } = (0, _cjsModuleLexer().parse)(transformedCode);
    const namedExports = new Set(exports);
    for (const reexport of reexports) {
      if (this.resolution.isCoreModule(reexport)) {
        const coreExports = this.loadCoreReexport(modulePath, reexport);
        if (coreExports !== null && typeof coreExports === 'object') {
          for (const key of Object.keys(coreExports)) namedExports.add(key);
        }
      } else {
        const resolved = this.resolution.resolveCjs(modulePath, reexport);
        for (const key of this.getExportsOf(modulePath, resolved)) namedExports.add(key);
      }
    }
    this.cache.set(modulePath, namedExports);
    return namedExports;
  }
  clear() {
    this.cache.clear();
  }
}
exports.CjsExportsCache = CjsExportsCache;

/***/ },

/***/ "./src/internals/CjsLoader.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.CjsLoader = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
var _ModuleExecutor = __webpack_require__("./src/internals/ModuleExecutor.ts");
var _nodeCapabilities = __webpack_require__("./src/internals/nodeCapabilities.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class CjsLoader {
  resolution;
  registries;
  mockState;
  transformCache;
  environment;
  coreModule;
  executor;
  requireEsm;
  testState;
  logFormattedReferenceError;
  constructor(options) {
    this.resolution = options.resolution;
    this.registries = options.registries;
    this.mockState = options.mockState;
    this.transformCache = options.transformCache;
    this.environment = options.environment;
    this.coreModule = options.coreModule;
    this.executor = options.executor;
    this.requireEsm = options.requireEsm;
    this.testState = options.testState;
    this.logFormattedReferenceError = options.logFormattedReferenceError;
  }
  requireModule(from, moduleName, options, isRequireActual = false) {
    const isInternal = options?.isInternalModule ?? false;
    const moduleID = this.mockState.getCjsModuleId(from, moduleName);
    let modulePath;

    // Some old tests rely on this mocking behavior. Ideally we'll change this
    // to be more explicit.
    const moduleResource = moduleName && this.resolution.getModule(moduleName);
    const manualMock = moduleName && this.resolution.getCjsMockModule(from, moduleName);
    if (!options?.isInternalModule && !isRequireActual && !moduleResource && manualMock && manualMock !== this.executor.getCurrentlyExecutingManualMock() && !this.mockState.isExplicitlyUnmocked(moduleID)) {
      modulePath = manualMock;
    }
    if (moduleName && this.resolution.isCoreModule(moduleName)) {
      return this.coreModule.require(moduleName, _nodeCapabilities.supportsNodeColonModulePrefixInRequire);
    }
    if (!modulePath) {
      modulePath = this.resolution.resolveCjs(from, moduleName);
    }
    if (this.resolution.shouldLoadAsEsm(modulePath)) {
      if (!_nodeCapabilities.supportsSyncEvaluate) {
        const error = new Error(`Must use import to load ES Module: ${modulePath}\n` + "Jest's require(ESM) requires Node v24.9+ for " + 'synchronous vm module APIs; the current Node version does not ' + 'expose them.');
        error.code = 'ERR_REQUIRE_ESM';
        throw error;
      }
      // Fast path: skip the graph walker on cache hits.
      const reg = this.registries.getActiveEsmRegistry();
      const cached = reg.get(modulePath);
      if (cached && !(cached instanceof Promise)) {
        return cached.namespace;
      }
      return this.requireEsm(modulePath);
    }
    const moduleRegistry = isInternal ? this.registries.getInternalCjsRegistry() : this.registries.getActiveCjsRegistry();
    const module = moduleRegistry.get(modulePath);
    if (module) {
      return module.exports;
    }

    // We must register the pre-allocated module object first so that any
    // circular dependencies that may arise while evaluating the module can
    // be satisfied.
    const localModule = {
      children: [],
      exports: {},
      filename: modulePath,
      id: modulePath,
      isPreloading: false,
      loaded: false,
      path: path().dirname(modulePath)
    };
    moduleRegistry.set(modulePath, localModule);
    try {
      this.loadModule(localModule, from, moduleName, modulePath, options, moduleRegistry);
    } catch (error) {
      moduleRegistry.delete(modulePath);
      // Mirror of `loadCjsAsEsm`'s SyntaxError fallback for `require()`.
      if (_nodeCapabilities.supportsSyncEvaluate && (0, _ModuleExecutor.isCjsParseError)(error)) {
        try {
          return this.requireEsm(modulePath);
        } catch (esmError) {
          if (esmError instanceof Error && esmError.name === 'SyntaxError') {
            throw error;
          }
          throw esmError;
        }
      }
      throw error;
    }
    return localModule.exports;
  }
  loadModule(localModule, from, moduleName, modulePath, options, moduleRegistry) {
    if (path().extname(modulePath) === '.json') {
      const transformed = this.transformCache.transformJson(modulePath, options);
      localModule.exports = this.environment.global.JSON.parse(transformed);
    } else if (path().extname(modulePath) === '.node') {
      localModule.exports = require(modulePath);
    } else {
      // testState gates apply only to executing JS bodies - JSON/.node go
      // through pure data parsing and don't run user code in the VM.
      if (this.testState.bailIfTornDown('You are trying to `require` a file after the Jest environment has been torn down.')) {
        return;
      }
      if (!_nodeCapabilities.runtimeSupportsVmModules) {
        this.testState.throwIfBetweenTests('You are trying to `require` a file outside of the scope of the test code.');
      }
      const fromPath = moduleName ? from : null;
      const result = this.executor.exec(localModule, options, moduleRegistry, fromPath, moduleName);
      if (result === 'env-disposed') {
        this.logFormattedReferenceError('You are trying to `require` a file after the Jest environment has been torn down.');
        process.exitCode = 1;
        return;
      }
    }
    localModule.loaded = true;
  }
}
exports.CjsLoader = CjsLoader;

/***/ },

/***/ "./src/internals/EsmLoader.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.EsmLoader = void 0;
exports.validateImportAttributes = validateImportAttributes;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _nodeUrl() {
  const data = require("node:url");
  _nodeUrl = function () {
    return data;
  };
  return data;
}
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
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
var _helpers = __webpack_require__("./src/helpers.ts");
var _ModuleExecutor = __webpack_require__("./src/internals/ModuleExecutor.ts");
var _Resolution = __webpack_require__("./src/internals/Resolution.ts");
var _nodeCapabilities = __webpack_require__("./src/internals/nodeCapabilities.ts");
var _syntheticBuilders = __webpack_require__("./src/internals/syntheticBuilders.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// `'sync-required'` is `require(esm)` (must be loaded synchronously, throw a
// typed error on edges that would normally bail). `'sync-preferred'` is the
// fast path for `await import()` (try sync; fall back to the legacy async
// loader on any unsupported edge).

// Shape of the third arg Node passes to the `module.link` callback. TC39 final
// is `{attributes}`; legacy was `{assert}`. `@types/node@18` only types the
// legacy field, so we declare both ourselves.
// TODO(jest next major): drop `assert` once we require Node 22+.

// Source-text entries carry their dep cacheKeys (used for `linkRequests`).
// Synthetic entries (mocks, core, JSON, wasm, @jest/globals) start linked
// and never appear in the link-requests pass.

// `SourceTextModule#hasAsyncGraph()` lets us prove a graph is sync-evaluable.
// `SyntheticModule` does not expose it but is by definition sync (the user
// callback is sync), so treat its absence as "not async".
function moduleHasAsyncGraph(module) {
  return typeof module.hasAsyncGraph === 'function' ? module.hasAsyncGraph() : false;
}

// Mirrors Node's `require(esm)` error code so user catches work uniformly.
function makeRequireAsyncError(modulePath, detail) {
  const error = new Error(`require() cannot be used to load ES Module ${modulePath}: ${detail}`);
  error.code = 'ERR_REQUIRE_ASYNC_MODULE';
  return error;
}

// Decode a `data:` URI specifier into its mime type and decoded code/body.
// `application/wasm` returns a Buffer; everything else returns a UTF-8 string.
const dataURIRegex = /^data:(?<mime>text\/javascript|application\/json|application\/wasm)(?:;(?<encoding>charset=utf-8|base64))?,(?<code>.*)$/;
function parseDataUri(specifier) {
  const match = specifier.match(dataURIRegex);
  if (!match || !match.groups) {
    throw new Error('Invalid data URI');
  }
  const {
    mime,
    encoding,
    code
  } = match.groups;
  if (mime === 'application/wasm') {
    if (!encoding) throw new Error('Missing data URI encoding');
    if (encoding !== 'base64') {
      throw new Error(`Invalid data URI encoding: ${encoding}`);
    }
    return {
      code: Buffer.from(code, 'base64'),
      mime
    };
  }
  if (!encoding || encoding === 'charset=utf-8') {
    return {
      code: decodeURIComponent(code),
      mime
    };
  }
  if (encoding === 'base64') {
    return {
      code: Buffer.from(code, 'base64').toString(),
      mime
    };
  }
  throw new Error(`Invalid data URI encoding: ${encoding}`);
}

// Mirrors Node's `validateAttributes` in lib/internal/modules/esm/assert.js.
// The only deliberate divergence: missing `type: 'json'` warns instead of
// throwing — see the JSON branch below.
const warnedMissingJsonAttributePairs = new Set();
// Soft cap so a long-lived process (watch mode, --runInBand) can't grow the
// set without bound. When we hit it we drop everything; users see at most one
// extra repeated warning per pair, which is benign.
const MAX_WARNED_PAIRS = 10_000;
function isJsonModule(modulePath) {
  return modulePath.endsWith('.json') || modulePath.startsWith('data:application/json');
}

// Avoid dumping the full payload of data: URIs (or other very long specifiers)
// into stderr.
function describeForWarning(modulePath) {
  if (modulePath.startsWith('data:')) {
    const comma = modulePath.indexOf(',');
    if (comma > 0) return `${modulePath.slice(0, comma)},…`;
  }
  return modulePath;
}
function makeImportAttributeError(code, message) {
  const error = new TypeError(message);
  error.code = code;
  return error;
}
function validateImportAttributes(modulePath, attributes, referencingIdentifier) {
  for (const key of Object.keys(attributes)) {
    if (key !== 'type') {
      throw makeImportAttributeError('ERR_IMPORT_ATTRIBUTE_UNSUPPORTED', `Import attribute "${key}" with value "${attributes[key]}" is not supported (importing "${modulePath}" from ${referencingIdentifier})`);
    }
  }
  const declaredType = attributes.type;
  const isJson = isJsonModule(modulePath);
  if (isJson) {
    if (declaredType === undefined) {
      // TODO(jest next major): match Node and throw
      // ERR_IMPORT_ATTRIBUTE_MISSING here. Until then, warn so existing users
      // without `with { type: 'json' }` keep working.
      const dedupeKey = `${referencingIdentifier}::${modulePath}`;
      if (!warnedMissingJsonAttributePairs.has(dedupeKey)) {
        if (warnedMissingJsonAttributePairs.size >= MAX_WARNED_PAIRS) {
          warnedMissingJsonAttributePairs.clear();
        }
        warnedMissingJsonAttributePairs.add(dedupeKey);
        const moduleLabel = describeForWarning(modulePath);
        console.warn('Jest: importing JSON without an import attribute is deprecated and will be a hard error in the next major. ' + `Update the import of "${moduleLabel}" (from ${referencingIdentifier}): ` + "use `with { type: 'json' }` for static imports, or pass " + "`{ with: { type: 'json' } }` as the second argument to dynamic `import()`.");
      }
      return;
    }
    if (declaredType !== 'json') {
      throw makeImportAttributeError('ERR_IMPORT_ATTRIBUTE_TYPE_INCOMPATIBLE', `Module "${modulePath}" is not of type "${declaredType}"`);
    }
    return;
  }

  // Non-JSON (implicit-type) module. Per HTML spec, the default type cannot
  // be re-asserted, so any explicit `type` attribute is rejected.
  if (declaredType !== undefined) {
    throw makeImportAttributeError('ERR_IMPORT_ATTRIBUTE_TYPE_INCOMPATIBLE', `Module "${modulePath}" is not of type "${declaredType}"`);
  }
}
const ESM_TRANSFORM_OPTIONS = {
  isInternalModule: false,
  supportsDynamicImport: true,
  supportsExportNamespaceFrom: true,
  supportsStaticESM: true,
  supportsTopLevelAwait: true
};
function stripFileScheme(specifier) {
  return specifier.startsWith('file://') ? (0, _nodeUrl().fileURLToPath)(specifier) : specifier;
}
class EsmLoader {
  resolution;
  fileCache;
  transformCache;
  registries;
  mockState;
  environment;
  cjsExportsCache;
  coreModule;
  jestGlobals;
  shouldLoadAsEsm;
  requireModuleOrMock;
  testState;
  // Used only by the legacy async path; deletable when min-Node ≥ v24.9
  // (delete the block at the bottom of this file too - eslint/tsc will
  // surface anything else that becomes unused).
  linkingMap = new WeakMap();
  evaluatingMap = new WeakMap();
  constructor(options) {
    this.resolution = options.resolution;
    this.fileCache = options.fileCache;
    this.transformCache = options.transformCache;
    this.registries = options.registries;
    this.mockState = options.mockState;
    this.environment = options.environment;
    this.cjsExportsCache = options.cjsExportsCache;
    this.coreModule = options.coreModule;
    this.jestGlobals = options.jestGlobals;
    this.shouldLoadAsEsm = options.shouldLoadAsEsm;
    this.requireModuleOrMock = options.requireModuleOrMock;
    this.testState = options.testState;
  }

  // A `null` here means the legacy async path is mid-flight on this same
  // module (registry holds a Promise from a concurrent `await import()`);
  // surface as ERR_REQUIRE_ESM with actionable context.
  //
  // Root-level mocks (`jest.unstable_mockModule(spec)` then `require(spec)`)
  // are not consulted - driving a SyntheticModule from `unlinked` to
  // `evaluated` needs the async link()/evaluate() pair. Transitive-dep mocks
  // still apply via the graph walker.
  requireEsmModule(modulePath) {
    const module = this.tryLoadGraphSync(modulePath, '', 'sync-required');
    if (!module) {
      const error = new Error(`Cannot require() ES Module ${modulePath} synchronously: it is currently being loaded by a concurrent \`import()\`. Await that import before calling require(), or import this module instead of requiring it.`);
      error.code = 'ERR_REQUIRE_ESM';
      throw error;
    }
    return module.namespace;
  }

  // Public for unit-test access. Production callers reach the sync graph
  // through `requireEsmModule` (sync require entry) or via `loadEsmModule`
  // (the legacy async entry, which first-tries this).
  tryLoadGraphSync(rootPath, rootQuery, mode) {
    if (this.testState.bailIfTornDown('You are trying to `import` a file after the Jest environment has been torn down.')) {
      return null;
    }
    const registry = this.registries.getActiveEsmRegistry();
    const rootKey = rootPath + rootQuery;
    const cached = registry.get(rootKey);
    if (cached) {
      if (cached instanceof Promise) return null;
      // The legacy `loadEsmModule` source-text branch does `registry.set`
      // while the `SourceTextModule` is still `'unlinked'` (link runs later
      // in `linkAndEvaluateModule`); accessing `.namespace` on a non-evaluated
      // module throws `ERR_VM_MODULE_STATUS`. Surface settled entries
      // (`'evaluated'` / `'errored'`); bail otherwise.
      if (cached.status === 'evaluated') return cached;
      if (cached.status === 'errored') throw cached.error;
      return null;
    }
    const context = this.getContext();
    if (this.transformCache.hasMutex(rootKey)) return null;
    const scratch = new Map();
    const worklist = [{
      cacheKey: rootKey,
      modulePath: rootPath
    }];
    while (worklist.length > 0) {
      const {
        cacheKey,
        modulePath
      } = worklist.pop();
      if (scratch.has(cacheKey)) continue;

      // Registry first, mutex second. Same settled-status gate as the root -
      // anything in `'unlinked'` / `'linking'` / `'linked'` / `'evaluating'`
      // is the legacy path mid-flight on this dep. Plugging an unlinked
      // module into the parent's `linkRequests` would fail Node's link
      // cascade; plugging a `'linked'` one would skip its body. Bail.
      const fromRegistry = registry.get(cacheKey);
      if (fromRegistry instanceof Promise) return null;
      if (fromRegistry) {
        if (fromRegistry.status === 'errored') throw fromRegistry.error;
        if (fromRegistry.status !== 'evaluated') return null;
        scratch.set(cacheKey, {
          cacheKey,
          kind: 'synthetic',
          module: fromRegistry
        });
        continue;
      }
      if (this.transformCache.hasMutex(cacheKey)) return null;
      if (this.resolution.isCoreModule(modulePath)) {
        scratch.set(cacheKey, {
          cacheKey,
          kind: 'synthetic',
          module: (0, _syntheticBuilders.buildCoreSyntheticModule)(modulePath, context, (name, prefix) => this.coreModule.require(name, prefix))
        });
        continue;
      }
      if (modulePath.startsWith('data:')) {
        const built = this.buildSyncDataUriEntry(modulePath, cacheKey, context, scratch, registry, worklist, mode);
        if (built === null) return null;
        scratch.set(cacheKey, built);
        continue;
      }
      if ((0, _Resolution.isWasm)(modulePath)) {
        const wasmEntry = this.buildSyncWasmEntry(this.fileCache.readFileBuffer(modulePath), modulePath, cacheKey, context, scratch, registry, worklist, mode);
        if (wasmEntry === null) return null;
        scratch.set(cacheKey, wasmEntry);
        continue;
      }
      if (!this.transformCache.canTransformSync(modulePath)) {
        if (mode === 'sync-required') {
          throw makeRequireAsyncError(modulePath, 'a configured transformer is async-only');
        }
        return null;
      }
      if (modulePath.endsWith('.json')) {
        scratch.set(cacheKey, {
          cacheKey,
          kind: 'synthetic',
          module: (0, _syntheticBuilders.buildJsonSyntheticModule)(this.transformCache.transform(modulePath, ESM_TRANSFORM_OPTIONS), modulePath, context)
        });
        continue;
      }
      const transformedCode = this.transformCache.transform(modulePath, ESM_TRANSFORM_OPTIONS);
      const module = new (_nodeVm().SourceTextModule)(transformedCode, {
        context,
        identifier: modulePath,
        importModuleDynamically: this.dynamicImport,
        initializeImportMeta: meta => {
          const metaUrl = (0, _nodeUrl().pathToFileURL)(modulePath).href;
          meta.url = metaUrl;
          // @ts-expect-error Jest uses @types/node@18.
          meta.filename = modulePath;
          // @ts-expect-error Jest uses @types/node@18.
          meta.dirname = path().dirname(modulePath);
          meta.resolve = (specifier, parent = metaUrl) => {
            const parentPath = (0, _nodeUrl().fileURLToPath)(parent);
            return (0, _nodeUrl().pathToFileURL)(this.resolution.resolveEsm(parentPath, specifier)).href;
          };
          meta.jest = this.jestGlobals.jestObjectFor(modulePath);
        }
      });
      if (typeof module.hasTopLevelAwait === 'function' && module.hasTopLevelAwait()) {
        if (mode === 'sync-required') {
          throw makeRequireAsyncError(modulePath, 'top-level await');
        }
        return null;
      }

      // If we got here without `moduleRequests`, the capability gate is lying.
      (0, _jestUtil().invariant)(module.moduleRequests !== undefined, `moduleRequests unavailable on ${modulePath}`);
      const deps = [];
      for (const {
        specifier,
        attributes
      } of module.moduleRequests) {
        const resolved = this.resolveSpecifierForSyncGraph(modulePath, specifier, context, scratch, registry, mode);
        if (resolved === null) return null;
        validateImportAttributes(resolved.modulePath, attributes, modulePath);
        deps.push(resolved.cacheKey);
        if (resolved.enqueue) worklist.push(resolved.enqueue);
      }
      scratch.set(cacheKey, {
        cacheKey,
        deps,
        kind: 'source',
        module
      });
    }
    for (const entry of scratch.values()) {
      if (entry.kind !== 'source') continue;
      const depModules = entry.deps.map(depKey => {
        const depEntry = scratch.get(depKey);
        (0, _jestUtil().invariant)(depEntry, `Sync ESM graph missing dep ${depKey} for ${entry.cacheKey}. This is a bug in Jest, please report it!`);
        return depEntry.module;
      });
      (0, _jestUtil().invariant)(typeof entry.module.linkRequests === 'function', `linkRequests unavailable on ${entry.cacheKey}`);
      entry.module.linkRequests(depModules);
    }
    const rootEntry = scratch.get(rootKey);
    (0, _jestUtil().invariant)(rootEntry, 'Sync ESM graph missing root entry');
    const rootModule = rootEntry.module;
    if (rootEntry.kind === 'source') {
      (0, _jestUtil().invariant)(typeof rootModule.instantiate === 'function', 'instantiate unavailable on root');
      rootModule.instantiate();
      if (moduleHasAsyncGraph(rootModule)) {
        if (mode === 'sync-required') {
          let culprit = rootModule.identifier;
          for (const entry of scratch.values()) {
            if (entry.kind === 'source' && typeof entry.module.hasTopLevelAwait === 'function' && entry.module.hasTopLevelAwait()) {
              culprit = entry.module.identifier;
              break;
            }
          }
          throw makeRequireAsyncError(rootModule.identifier, culprit === rootModule.identifier ? 'top-level await' : `a dependency uses top-level await (${culprit})`);
        }
        return null;
      }
    }
    for (const entry of scratch.values()) {
      if (!registry.has(entry.cacheKey)) {
        registry.set(entry.cacheKey, entry.module);
      }
    }
    rootModule.evaluate().catch(_helpers.noop);
    if (rootModule.status === 'errored') {
      throw rootModule.error;
    }
    (0, _jestUtil().invariant)(rootModule.status === 'evaluated', `Expected synchronous evaluation to complete for ${rootModule.identifier}, but module status is "${rootModule.status}". This is a bug in Jest, please report it!`);
    return rootModule;
  }
  getContext() {
    (0, _jestUtil().invariant)(typeof this.environment.getVmContext === 'function', 'ES Modules are only supported if your test environment has the `getVmContext` function');
    const context = this.environment.getVmContext();
    (0, _jestUtil().invariant)(context, 'Test environment has been torn down');
    return context;
  }

  // Commits (or reuses) a synthetic-module entry under `cacheKey` in both the
  // local scratch and the long-lived registry. Returns `false` when the
  // registry holds something the caller must bail on: a mid-flight Promise
  // from the legacy async path, or a non-evaluated module (legacy can stash
  // an `'unlinked'` SourceTextModule here while link/evaluate runs).
  tryCommitSynthetic(cacheKey, registry, scratch, build) {
    if (scratch.has(cacheKey)) return true;
    const fromRegistry = registry.get(cacheKey);
    if (fromRegistry instanceof Promise) return false;
    if (fromRegistry) {
      const cached = fromRegistry;
      if (cached.status === 'errored') throw cached.error;
      if (cached.status !== 'evaluated') return false;
    }
    const module = fromRegistry ?? build();
    if (!fromRegistry) registry.set(cacheKey, module);
    scratch.set(cacheKey, {
      cacheKey,
      kind: 'synthetic',
      module
    });
    return true;
  }
  resolveSpecifierForSyncGraph(referencingIdentifier, specifier, context, scratch, registry, mode) {
    if (specifier === '@jest/globals') {
      const cacheKey = `@jest/globals/${referencingIdentifier}`;
      const ok = this.tryCommitSynthetic(cacheKey, registry, scratch, () => this.jestGlobals.esmGlobalsModule(referencingIdentifier, context));
      return ok ? {
        cacheKey,
        enqueue: null,
        modulePath: cacheKey
      } : null;
    }
    if (specifier.startsWith('data:')) {
      const cacheKey = specifier;
      return {
        cacheKey,
        enqueue: {
          cacheKey,
          modulePath: specifier
        },
        modulePath: specifier
      };
    }
    specifier = stripFileScheme(specifier);
    const [specifierPath, query = ''] = specifier.split('?');
    const {
      shouldMock,
      moduleID
    } = this.mockState.shouldMockEsmSync(referencingIdentifier, specifierPath);
    if (shouldMock) {
      const mocked = this.importMockSync(specifierPath, moduleID, context, scratch, mode);
      if (mocked === null) return null;
      return {
        cacheKey: mocked.cacheKey,
        enqueue: null,
        modulePath: specifierPath
      };
    }
    if (this.resolution.isCoreModule(specifierPath)) {
      const cacheKey = specifierPath + query;
      return {
        cacheKey,
        enqueue: {
          cacheKey,
          modulePath: specifierPath
        },
        modulePath: specifierPath
      };
    }
    let resolved;
    try {
      resolved = this.resolution.resolveEsm(referencingIdentifier, specifierPath);
    } catch (error) {
      if (mode === 'sync-required') throw error;
      return null;
    }
    const cacheKey = resolved + query;
    if (!resolved.endsWith('.json') && !(0, _Resolution.isWasm)(resolved) && !this.shouldLoadAsEsm(resolved)) {
      const ok = this.tryCommitSynthetic(cacheKey, registry, scratch, () => this.buildCjsAsEsmSyntheticModule(referencingIdentifier, resolved, context));
      return ok ? {
        cacheKey,
        enqueue: null,
        modulePath: resolved
      } : null;
    }
    return {
      cacheKey,
      enqueue: {
        cacheKey,
        modulePath: resolved
      },
      modulePath: resolved
    };
  }
  importMockSync(moduleName, moduleID, context, scratch, mode) {
    const existing = this.registries.getModuleMock(moduleID);
    if (existing instanceof Promise) return null;
    if (existing) {
      if (existing.status === 'errored') throw existing.error;
      if (!scratch.has(moduleID)) {
        scratch.set(moduleID, {
          cacheKey: moduleID,
          kind: 'synthetic',
          module: existing
        });
      }
      return {
        cacheKey: moduleID
      };
    }
    const factory = this.mockState.getEsmFactory(moduleID);
    // `shouldMockEsmSync` said this spec is mocked but no factory was
    // registered.
    (0, _jestUtil().invariant)(factory !== undefined, 'Attempting to import a mock without a factory');
    const result = factory();
    if ((0, _jestUtil().isPromise)(result)) {
      if (mode === 'sync-required') {
        throw makeRequireAsyncError(moduleName, 'mock factory is async');
      }
      return null;
    }
    const synth = (0, _syntheticBuilders.syntheticFromExports)(moduleName, context, result);
    this.registries.setModuleMock(moduleID, synth);
    scratch.set(moduleID, {
      cacheKey: moduleID,
      kind: 'synthetic',
      module: synth
    });
    return {
      cacheKey: moduleID
    };
  }

  // Construct a wasm SyntheticModule for the sync graph. Wasm imports are
  // resolved (sync) and enqueued like static-import deps. The SyntheticModule's
  // body closure-captures `scratch`; by evaluate-cascade time, every dep entry
  // is fully evaluated so `module.namespace` is safe to read.
  //
  // Uses `new WebAssembly.Module(bytes)` (sync, blocks on large modules).
  buildSyncWasmEntry(bytes, identifier, cacheKey, context, scratch, registry, worklist, mode) {
    const wasmModule = new WebAssembly.Module(bytes);
    const moduleSpecToCacheKey = new Map();
    for (const {
      module: depSpec
    } of WebAssembly.Module.imports(wasmModule)) {
      if (moduleSpecToCacheKey.has(depSpec)) continue;
      const resolved = this.resolveSpecifierForSyncGraph(identifier, depSpec, context, scratch, registry, mode);
      if (resolved === null) return null;
      moduleSpecToCacheKey.set(depSpec, resolved.cacheKey);
      if (resolved.enqueue) worklist.push(resolved.enqueue);
    }
    const synthetic = (0, _syntheticBuilders.buildWasmSyntheticModule)(wasmModule, identifier, context, depSpec => {
      const depKey = moduleSpecToCacheKey.get(depSpec);
      const depEntry = scratch.get(depKey);
      return depEntry.module.namespace;
    });
    return {
      cacheKey,
      kind: 'synthetic',
      module: synthetic
    };
  }
  buildSyncDataUriEntry(specifier, cacheKey, context, scratch, registry, worklist, mode) {
    const esmDynamicImport = this.dynamicImport;
    const {
      mime,
      code
    } = parseDataUri(specifier);
    if (mime === 'application/wasm') {
      return this.buildSyncWasmEntry(new Uint8Array(code), specifier, cacheKey, context, scratch, registry, worklist, mode);
    }
    if (mime === 'application/json') {
      return {
        cacheKey,
        kind: 'synthetic',
        module: (0, _syntheticBuilders.buildJsonSyntheticModule)(code, specifier, context)
      };
    }
    const module = new (_nodeVm().SourceTextModule)(code, {
      context,
      identifier: specifier,
      importModuleDynamically: esmDynamicImport,
      initializeImportMeta(meta) {
        meta.url = specifier;
        if (meta.url.startsWith('file://')) {
          // @ts-expect-error Jest uses @types/node@18.
          meta.filename = (0, _nodeUrl().fileURLToPath)(meta.url);
          // @ts-expect-error Jest uses @types/node@18.
          meta.dirname = path().dirname(meta.filename);
        }
      }
    });
    if (typeof module.hasTopLevelAwait === 'function' && module.hasTopLevelAwait()) {
      if (mode === 'sync-required') {
        throw makeRequireAsyncError(specifier, 'top-level await');
      }
      return null;
    }
    (0, _jestUtil().invariant)(module.moduleRequests !== undefined, `moduleRequests unavailable on ${specifier}`);
    const deps = [];
    for (const {
      specifier: depSpec,
      attributes
    } of module.moduleRequests) {
      const resolved = this.resolveSpecifierForSyncGraph(specifier, depSpec, context, scratch, registry, mode);
      if (resolved === null) return null;
      validateImportAttributes(resolved.modulePath, attributes, specifier);
      deps.push(resolved.cacheKey);
      if (resolved.enqueue) worklist.push(resolved.enqueue);
    }
    return {
      cacheKey,
      deps,
      kind: 'source',
      module
    };
  }

  // Synthetic-module wrappers that close over the primitive deps. The
  // `requireModuleOrMock` callback inside `buildCjsAsEsmSyntheticModule`
  // is the extension-point bridge to `Runtime.requireModuleOrMock`.
  buildCjsAsEsmSyntheticModule(from, modulePath, context) {
    return (0, _syntheticBuilders.buildCjsAsEsmSyntheticModule)(from, modulePath, context, this.requireModuleOrMock, this.cjsExportsCache);
  }

  // TODO: legacy async path - everything below is deletable when min-Node
  // ≥ v24.9 (the sync core handles all entry shapes). Drop the `linkingMap`
  // / `evaluatingMap` fields with it.

  // Called from CJS bodies via `compileFunction`'s `importModuleDynamically`.
  dynamicImportFromCjs(specifier, identifier, context, importAttributes) {
    return this.resolveModule(specifier, identifier, context).then(m => {
      validateImportAttributes(m.identifier, importAttributes ?? {}, identifier);
      return this.linkAndEvaluateModule(m);
    });
  }

  // Public entry for `Runtime.unstable_importModule`. Runtime keeps the
  // public method as the override seam; this is the body.
  async loadAndEvaluate(from, moduleName) {
    (0, _jestUtil().invariant)(_nodeCapabilities.runtimeSupportsVmModules, 'You need to run with a version of node that supports ES Modules in the VM API. See https://jestjs.io/docs/ecmascript-modules');
    const [specifierPath, query] = (moduleName ?? '').split('?');
    const modulePath = await this.resolution.resolveEsmAsync(from, specifierPath);
    const module = await this.loadEsmModule(modulePath, query);
    return this.linkAndEvaluateModule(module);
  }
  async loadEsmModule(modulePath, query = '') {
    // Two gates here. `supportsSyncEvaluate` is a Node-version check: the
    // sync core relies on `SyntheticModule` starting `'linked'` and on
    // `evaluate()` completing sync, both of which need v22.21+ / v24.8+.
    // `canResolveSync` is a configured-resolver check: with an async-only
    // user resolver `findNodeModule` silently falls back to the default
    // resolver and would silently miss user mappings.
    if (_nodeCapabilities.supportsSyncEvaluate && this.resolution.canResolveSync()) {
      const synced = this.tryLoadGraphSync(modulePath, query, 'sync-preferred');
      if (synced) return synced;
    }
    const cacheKey = modulePath + query;
    const registry = this.registries.getActiveEsmRegistry();
    if (this.transformCache.hasMutex(cacheKey)) {
      await this.transformCache.awaitMutex(cacheKey);
    }
    if (!registry.has(cacheKey)) {
      const context = this.getContext();
      let transformResolve;
      let transformReject;
      this.transformCache.setMutex(cacheKey, new Promise((resolve, reject) => {
        transformResolve = resolve;
        transformReject = reject;
      }));
      (0, _jestUtil().invariant)(transformResolve && transformReject, 'Promise initialization should be sync - please report this bug to Jest!');
      try {
        if ((0, _Resolution.isWasm)(modulePath)) {
          const wasm = this.importWasmModule(this.fileCache.readFileBuffer(modulePath), modulePath, context);
          registry.set(cacheKey, wasm);
          transformResolve();
          return wasm;
        }
        if (this.resolution.isCoreModule(modulePath)) {
          const core = (0, _syntheticBuilders.evaluateSyntheticModule)((0, _syntheticBuilders.buildCoreSyntheticModule)(modulePath, context, (name, prefix) => this.coreModule.require(name, prefix)));
          registry.set(cacheKey, core);
          transformResolve();
          return core;
        }
        const transformedCode = this.transformCache.canTransformSync(modulePath) ? this.transformCache.transform(modulePath, ESM_TRANSFORM_OPTIONS) : await this.transformCache.transformAsync(modulePath, ESM_TRANSFORM_OPTIONS);
        let module;
        if (modulePath.endsWith('.json')) {
          module = (0, _syntheticBuilders.buildJsonSyntheticModule)(transformedCode, modulePath, context);
        } else {
          module = new (_nodeVm().SourceTextModule)(transformedCode, {
            context,
            identifier: modulePath,
            importModuleDynamically: this.dynamicImport,
            initializeImportMeta: meta => {
              const metaUrl = (0, _nodeUrl().pathToFileURL)(modulePath).href;
              meta.url = metaUrl;
              // @ts-expect-error Jest uses @types/node@18.
              meta.filename = modulePath;
              // @ts-expect-error Jest uses @types/node@18.
              meta.dirname = path().dirname(modulePath);
              meta.resolve = (specifier, parent = metaUrl) => {
                const parentPath = (0, _nodeUrl().fileURLToPath)(parent);
                return (0, _nodeUrl().pathToFileURL)(this.resolution.resolveEsm(parentPath, specifier)).href;
              };
              meta.jest = this.jestGlobals.jestObjectFor(modulePath);
            }
          });
        }
        (0, _jestUtil().invariant)(!registry.has(cacheKey), `Module cache already has entry ${cacheKey}. This is a bug in Jest, please report it!`);
        registry.set(cacheKey, module);
        transformResolve();
      } catch (error) {
        transformReject(error);
        throw error;
      } finally {
        this.transformCache.clearMutex(cacheKey);
      }
    }
    const module = registry.get(cacheKey);
    (0, _jestUtil().invariant)(module, 'Module cache does not contain module. This is a bug in Jest, please open up an issue');
    return module;
  }
  async resolveModule(specifier, referencingIdentifier, context) {
    if (this.testState.bailIfTornDown('You are trying to `import` a file after the Jest environment has been torn down.')) {
      // @ts-expect-error -- exiting
      return;
    }
    const registry = this.registries.getActiveEsmRegistry();
    if (specifier === '@jest/globals') {
      const globalsIdentifier = `@jest/globals/${referencingIdentifier}`;
      const fromCache = registry.get(globalsIdentifier);
      if (fromCache) {
        return fromCache;
      }
      const globals = (0, _syntheticBuilders.evaluateSyntheticModule)(this.jestGlobals.esmGlobalsModule(referencingIdentifier, context));
      registry.set(globalsIdentifier, globals);
      return globals;
    }
    if (specifier.startsWith('data:')) {
      const dataDecision = await this.mockState.shouldMockEsmAsync(referencingIdentifier, specifier);
      if (dataDecision.shouldMock) {
        return this.importMock(specifier, dataDecision.moduleID, context);
      }
      const fromCache = registry.get(specifier);
      if (fromCache) {
        return fromCache;
      }
      const {
        mime,
        code
      } = parseDataUri(specifier);
      let module;
      if (mime === 'application/wasm') {
        module = await this.importWasmModule(new Uint8Array(code), specifier, context);
      } else if (mime === 'application/json') {
        module = (0, _syntheticBuilders.buildJsonSyntheticModule)(code, specifier, context);
      } else {
        module = new (_nodeVm().SourceTextModule)(code, {
          context,
          identifier: specifier,
          importModuleDynamically: this.dynamicImport,
          initializeImportMeta(meta) {
            meta.url = specifier;
            if (meta.url.startsWith('file://')) {
              // @ts-expect-error Jest uses @types/node@18.
              meta.filename = (0, _nodeUrl().fileURLToPath)(meta.url);
              // @ts-expect-error Jest uses @types/node@18.
              meta.dirname = path().dirname(meta.filename);
            }
          }
        });
      }
      registry.set(specifier, module);
      return module;
    }
    if (specifier.startsWith('file://')) {
      specifier = (0, _nodeUrl().fileURLToPath)(specifier);
    }
    const [specifierPath, query] = specifier.split('?');
    const decision = await this.mockState.shouldMockEsmAsync(referencingIdentifier, specifierPath);
    if (decision.shouldMock) {
      return this.importMock(specifierPath, decision.moduleID, context);
    }
    const resolved = await this.resolution.resolveEsmAsync(referencingIdentifier, specifierPath);
    if (resolved.endsWith('.json') || this.resolution.isCoreModule(resolved) || this.shouldLoadAsEsm(resolved)) {
      return this.loadEsmModule(resolved, query);
    }
    return this.loadCjsAsEsm(referencingIdentifier, resolved, context);
  }
  async linkAndEvaluateModule(module) {
    if (this.testState.bailIfTornDown('You are trying to `import` a file after the Jest environment has been torn down.')) {
      // @ts-expect-error: exiting early
      return;
    }

    // Already-errored module from a prior failed evaluation.
    if (module.status === 'errored') {
      throw module.error;
    }
    if (module.status === 'unlinked') {
      this.linkingMap.set(module, module.link(async (specifier, referencingModule, extra) => {
        const resolved = await this.resolveModule(specifier, referencingModule.identifier, referencingModule.context);
        const extraAttrs = extra;
        validateImportAttributes(resolved.identifier, extraAttrs?.attributes ?? extraAttrs?.assert ?? {}, referencingModule.identifier);
        return resolved;
      }));
    }
    const linkPromise = this.linkingMap.get(module);
    if (linkPromise != null) {
      await linkPromise;
    } else if (module.status === 'linking') {
      // Module entered 'linking' via Node's cascade (a parent's link()
      // recursed into this dep without going through our code). We have no
      // promise to await, so yield via setImmediate - which lets all pending
      // microtasks (including Node's internal linker chain) drain - until
      // linking finishes.
      const deadline = Date.now() + 5000;
      while (module.status === 'linking') {
        if (Date.now() > deadline) {
          throw new Error(`Jest: module ${module.identifier} is stuck in 'linking' state after 5 s - ` + 'this is likely a bug in Jest (please report it).');
        }
        await new Promise(resolve => setImmediate(resolve));
      }
    }
    if (module.status === 'linked') {
      if (_nodeCapabilities.supportsSyncEvaluate && !moduleHasAsyncGraph(module)) {
        // `evaluate()` fulfills synchronously when the graph has no top-level
        // await, so we don't need to yield. Errors land on `module.status`,
        // not as a Promise rejection. Gated on `supportsSyncEvaluate` because
        // pre-v22.21 / pre-v24.8 Node returns a genuinely-async Promise here
        // and the status invariant below would fire on `'evaluating'`.
        void module.evaluate().catch(_helpers.noop);
        const status = module.status;
        if (status === 'errored') {
          throw module.error;
        }
        (0, _jestUtil().invariant)(status === 'evaluated', `Expected synchronous evaluation to complete for ${module.identifier}, but module status is "${status}". This is a bug in Jest, please report it!`);
      } else {
        // Async path: TLA somewhere in the graph, or Node lacks the v22.21+ /
        // v24.8+ sync-evaluate semantics. Store the promise so concurrent
        // callers finding the module in `'evaluating'` await the same one.
        this.evaluatingMap.set(module, module.evaluate());
      }
    }
    await this.evaluatingMap.get(module);
    return module;
  }
  loadCjsAsEsm(from, modulePath, context) {
    const registry = this.registries.getActiveEsmRegistry();
    const cached = registry.get(modulePath);
    if (cached) {
      return cached;
    }
    let synthetic;
    try {
      synthetic = this.buildCjsAsEsmSyntheticModule(from, modulePath, context);
    } catch (error) {
      if (!(0, _ModuleExecutor.isCjsParseError)(error)) {
        throw error;
      }
      // The file may contain ESM syntax with no ESM marker (.mjs /
      // "type":"module") - try loading as native ESM. If the ESM parser also
      // rejects it, the original CJS error was the genuine one.
      return this.loadEsmModule(modulePath).catch(esmError => {
        throw (0, _jestUtil().isError)(esmError) && esmError.name === 'SyntaxError' ? error : esmError;
      });
    }
    const evaluated = (0, _syntheticBuilders.evaluateSyntheticModule)(synthetic);
    registry.set(modulePath, evaluated);
    return evaluated;
  }
  async importMock(moduleName, moduleID, context) {
    if (this.registries.hasModuleMock(moduleID)) {
      return this.registries.getModuleMock(moduleID);
    }
    const factory = this.mockState.getEsmFactory(moduleID);
    if (factory) {
      const invokedFactory = await factory();
      const module = (0, _syntheticBuilders.syntheticFromExports)(moduleName, context, invokedFactory);
      this.registries.setModuleMock(moduleID, module);
      return (0, _syntheticBuilders.evaluateSyntheticModule)(module);
    }
    throw new Error('Attempting to import a mock without a factory');
  }
  async importWasmModule(source, identifier, context) {
    // Use async `WebAssembly.compile` (rather than the sync constructor used
    // by the v24.9+ sync core) to avoid blocking the event loop on large wasm
    // modules in the legacy async path.
    const wasmModule = await WebAssembly.compile(source);
    const moduleLookup = {};
    for (const {
      module
    } of WebAssembly.Module.imports(wasmModule)) {
      if (moduleLookup[module] === undefined) {
        const resolvedModule = await this.resolveModule(module, identifier, context);
        // Do NOT call linkAndEvaluateModule here: we are executing inside the
        // linker callback for the parent module, so Node's cascade may already
        // be linking resolvedModule. Calling linkAndEvaluateModule would
        // spin-wait via setImmediate, but the cascade can't finish until this
        // linker returns - deadlock. The SyntheticModule's body runs only
        // after Node has fully evaluated all deps in topological order.
        moduleLookup[module] = resolvedModule;
      }
    }
    return (0, _syntheticBuilders.buildWasmSyntheticModule)(wasmModule, identifier, context, depSpec => moduleLookup[depSpec].namespace);
  }

  // Shared async dynamic-import callback installed on every SourceTextModule
  // we construct. Goes through the legacy async path; revisit when min-Node
  // reaches v24.9 (Node may handle dynamic imports for us by then).
  dynamicImport = async (specifier, referencingModule, importAttributes) => {
    (0, _jestUtil().invariant)(_nodeCapabilities.runtimeSupportsVmModules, 'You need to run with a version of node that supports ES Modules in the VM API. See https://jestjs.io/docs/ecmascript-modules');
    this.testState.throwIfBetweenTests('You are trying to `import` a file outside of the scope of the test code.');
    this.testState.throwIfTornDown('You are trying to `import` a file after the Jest environment has been torn down.');
    const dyn = await this.resolveModule(specifier, referencingModule.identifier, referencingModule.context);
    validateImportAttributes(dyn.identifier, importAttributes ?? {}, referencingModule.identifier);
    return this.linkAndEvaluateModule(dyn);
  };
}
exports.EsmLoader = EsmLoader;

/***/ },

/***/ "./src/internals/FileCache.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.FileCache = void 0;
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
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

class FileCache {
  strings;
  buffers = new Map();
  constructor(cacheFS) {
    this.strings = cacheFS;
  }
  readFile(filename) {
    let source = this.strings.get(filename);
    if (!source) {
      source = this.readFileBuffer(filename).toString();
      this.strings.set(filename, source);
    }
    return source;
  }
  readFileBuffer(filename) {
    let source = this.buffers.get(filename);
    if (!source) {
      source = fs().readFileSync(filename);
      this.buffers.set(filename, source);
    }
    return source;
  }
  clear() {
    this.strings.clear();
    this.buffers.clear();
  }
}
exports.FileCache = FileCache;

/***/ },

/***/ "./src/internals/JestGlobals.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.JestGlobals = void 0;
var _syntheticBuilders = __webpack_require__("./src/internals/syntheticBuilders.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const testTimeoutSymbol = Symbol.for('TEST_TIMEOUT_SYMBOL');
const retryTimesSymbol = Symbol.for('RETRY_TIMES');
const waitBeforeRetrySymbol = Symbol.for('WAIT_BEFORE_RETRY');
const retryImmediatelySymbol = Symbol.for('RETRY_IMMEDIATELY');
const logErrorsBeforeRetrySymbol = Symbol.for('LOG_ERRORS_BEFORE_RETRY');
class JestGlobals {
  config;
  globalConfig;
  environment;
  mockState;
  moduleMocker;
  setMockBridge;
  setModuleMockBridge;
  generateMock;
  requireActualBridge;
  requireMockBridge;
  resetModulesBridge;
  isolateModulesBridge;
  isolateModulesAsyncBridge;
  clearAllMocksBridge;
  resetAllMocksBridge;
  restoreAllMocksBridge;
  testState;
  logFormattedReferenceError;
  cache = new Map();
  fakeTimersImpl;
  envGlobalsOverride;
  cachedEnvGlobals;
  constructor(options) {
    this.config = options.config;
    this.globalConfig = options.globalConfig;
    this.environment = options.environment;
    this.mockState = options.mockState;
    this.moduleMocker = options.moduleMocker;
    this.setMockBridge = options.setMock;
    this.setModuleMockBridge = options.setModuleMock;
    this.generateMock = options.generateMock;
    this.requireActualBridge = options.requireActual;
    this.requireMockBridge = options.requireMock;
    this.resetModulesBridge = options.resetModules;
    this.isolateModulesBridge = options.isolateModules;
    this.isolateModulesAsyncBridge = options.isolateModulesAsync;
    this.clearAllMocksBridge = options.clearAllMocks;
    this.resetAllMocksBridge = options.resetAllMocks;
    this.restoreAllMocksBridge = options.restoreAllMocks;
    this.testState = options.testState;
    this.logFormattedReferenceError = options.logFormattedReferenceError;
    this.fakeTimersImpl = this.config.fakeTimers.legacyFakeTimers ? this.environment.fakeTimers : this.environment.fakeTimersModern;
  }
  jestObjectFor(from) {
    const cached = this.cache.get(from);
    if (cached) return cached;
    const fresh = this.buildJestObject(from);
    this.cache.set(from, fresh);
    return fresh;
  }
  envGlobals() {
    if (this.envGlobalsOverride) {
      return {
        ...this.envGlobalsOverride
      };
    }
    let cached = this.cachedEnvGlobals;
    if (cached === undefined) {
      cached = {
        afterAll: this.environment.global.afterAll,
        afterEach: this.environment.global.afterEach,
        beforeAll: this.environment.global.beforeAll,
        beforeEach: this.environment.global.beforeEach,
        describe: this.environment.global.describe,
        expect: this.environment.global.expect,
        fdescribe: this.environment.global.fdescribe,
        fit: this.environment.global.fit,
        it: this.environment.global.it,
        test: this.environment.global.test,
        xdescribe: this.environment.global.xdescribe,
        xit: this.environment.global.xit,
        xtest: this.environment.global.xtest
      };
      this.cachedEnvGlobals = cached;
    }
    return {
      ...cached
    };
  }
  cjsGlobals(from) {
    return {
      ...this.envGlobals(),
      jest: this.jestObjectFor(from)
    };
  }
  esmGlobalsModule(from, context) {
    return (0, _syntheticBuilders.syntheticFromExports)('@jest/globals', context, this.cjsGlobals(from));
  }
  setEnvGlobalsOverride(globals) {
    this.envGlobalsOverride = globals;
  }
  clearJestObjectCache() {
    this.cache.clear();
  }
  buildJestObject(from) {
    const disableAutomock = () => {
      this.mockState.disableAutomock();
      return jestObject;
    };
    const enableAutomock = () => {
      this.mockState.enableAutomock();
      return jestObject;
    };
    const unmock = moduleName => {
      this.mockState.unmockCjs(from, moduleName);
      return jestObject;
    };
    const unmockModule = moduleName => {
      this.mockState.unmockEsm(from, moduleName);
      return jestObject;
    };
    const deepUnmock = moduleName => {
      this.mockState.deepUnmock(from, moduleName);
      return jestObject;
    };
    const mock = (moduleName, mockFactory, options) => {
      if (mockFactory !== undefined) {
        return setMockFactory(moduleName, mockFactory, options);
      }
      this.mockState.markExplicitCjsMock(from, moduleName);
      return jestObject;
    };
    const onGenerateMock = cb => {
      this.mockState.addOnGenerateMock(cb);
      return jestObject;
    };
    const setMockFactory = (moduleName, mockFactory, options) => {
      this.setMockBridge(from, moduleName, mockFactory, options);
      return jestObject;
    };
    const mockModule = (moduleName, mockFactory, options) => {
      if (typeof mockFactory !== 'function') {
        throw new TypeError('`unstable_mockModule` must be passed a mock factory');
      }
      this.setModuleMockBridge(from, moduleName, mockFactory, options);
      return jestObject;
    };
    const clearAllMocks = () => {
      this.clearAllMocksBridge();
      return jestObject;
    };
    const resetAllMocks = () => {
      this.resetAllMocksBridge();
      return jestObject;
    };
    const restoreAllMocks = () => {
      this.restoreAllMocksBridge();
      return jestObject;
    };
    const _getFakeTimers = () => {
      if (this.testState.isTornDown() || !(this.environment.fakeTimers || this.environment.fakeTimersModern)) {
        this.logFormattedReferenceError('You are trying to access a property or method of the Jest environment after it has been torn down.');
        process.exitCode = 1;
      }
      this.testState.throwIfBetweenTests('You are trying to access a property or method of the Jest environment outside of the scope of the test code.');
      return this.fakeTimersImpl;
    };
    const useFakeTimers = fakeTimersConfig => {
      fakeTimersConfig = {
        ...this.config.fakeTimers,
        ...fakeTimersConfig
      };
      if (fakeTimersConfig?.legacyFakeTimers) {
        this.fakeTimersImpl = this.environment.fakeTimers;
      } else {
        this.fakeTimersImpl = this.environment.fakeTimersModern;
      }
      this.fakeTimersImpl.useFakeTimers(fakeTimersConfig);
      return jestObject;
    };
    const useRealTimers = () => {
      _getFakeTimers().useRealTimers();
      return jestObject;
    };
    const resetModules = () => {
      this.resetModulesBridge();
      return jestObject;
    };
    const isolateModules = fn => {
      this.isolateModulesBridge(fn);
      return jestObject;
    };
    const isolateModulesAsync = this.isolateModulesAsyncBridge;
    const fn = this.moduleMocker.fn.bind(this.moduleMocker);
    const spyOn = this.moduleMocker.spyOn.bind(this.moduleMocker);
    const mocked = this.moduleMocker.mocked.bind(this.moduleMocker);
    const replaceProperty = this.moduleMocker.replaceProperty.bind(this.moduleMocker);
    const setTimeout = timeout => {
      this.environment.global[testTimeoutSymbol] = timeout;
      return jestObject;
    };
    const retryTimes = (numTestRetries, options) => {
      this.environment.global[retryTimesSymbol] = numTestRetries;
      this.environment.global[logErrorsBeforeRetrySymbol] = options?.logErrorsBeforeRetry;
      this.environment.global[waitBeforeRetrySymbol] = options?.waitBeforeRetry;
      this.environment.global[retryImmediatelySymbol] = options?.retryImmediately;
      return jestObject;
    };
    const jestObject = {
      advanceTimersByTime: msToRun => _getFakeTimers().advanceTimersByTime(msToRun),
      advanceTimersByTimeAsync: async msToRun => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          await fakeTimers.advanceTimersByTimeAsync(msToRun);
        } else {
          throw new TypeError('`jest.advanceTimersByTimeAsync()` is not available when using legacy fake timers.');
        }
      },
      advanceTimersToNextFrame: () => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          return fakeTimers.advanceTimersToNextFrame();
        }
        throw new TypeError('`jest.advanceTimersToNextFrame()` is not available when using legacy fake timers.');
      },
      advanceTimersToNextTimer: steps => _getFakeTimers().advanceTimersToNextTimer(steps),
      advanceTimersToNextTimerAsync: async steps => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          await fakeTimers.advanceTimersToNextTimerAsync(steps);
        } else {
          throw new TypeError('`jest.advanceTimersToNextTimerAsync()` is not available when using legacy fake timers.');
        }
      },
      autoMockOff: disableAutomock,
      autoMockOn: enableAutomock,
      clearAllMocks,
      clearAllTimers: () => _getFakeTimers().clearAllTimers(),
      createMockFromModule: moduleName => this.generateMock(from, moduleName),
      deepUnmock,
      disableAutomock,
      doMock: mock,
      dontMock: unmock,
      enableAutomock,
      fn,
      getRealSystemTime: () => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          return fakeTimers.getRealSystemTime();
        } else {
          throw new TypeError('`jest.getRealSystemTime()` is not available when using legacy fake timers.');
        }
      },
      getSeed: () => this.globalConfig.seed,
      getTimerCount: () => _getFakeTimers().getTimerCount(),
      isEnvironmentTornDown: () => this.testState.isTornDown(),
      isMockFunction: this.moduleMocker.isMockFunction,
      isolateModules,
      isolateModulesAsync,
      mock,
      mocked,
      now: () => _getFakeTimers().now(),
      onGenerateMock,
      replaceProperty,
      requireActual: moduleName => this.requireActualBridge(from, moduleName),
      requireMock: moduleName => this.requireMockBridge(from, moduleName),
      resetAllMocks,
      resetModules,
      restoreAllMocks,
      retryTimes,
      runAllImmediates: () => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimers) {
          fakeTimers.runAllImmediates();
        } else {
          throw new TypeError('`jest.runAllImmediates()` is only available when using legacy fake timers.');
        }
      },
      runAllTicks: () => _getFakeTimers().runAllTicks(),
      runAllTimers: () => _getFakeTimers().runAllTimers(),
      runAllTimersAsync: async () => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          await fakeTimers.runAllTimersAsync();
        } else {
          throw new TypeError('`jest.runAllTimersAsync()` is not available when using legacy fake timers.');
        }
      },
      runOnlyPendingTimers: () => _getFakeTimers().runOnlyPendingTimers(),
      runOnlyPendingTimersAsync: async () => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          await fakeTimers.runOnlyPendingTimersAsync();
        } else {
          throw new TypeError('`jest.runOnlyPendingTimersAsync()` is not available when using legacy fake timers.');
        }
      },
      setMock: (moduleName, mock) => setMockFactory(moduleName, () => mock),
      setSystemTime: now => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          fakeTimers.setSystemTime(now);
        } else {
          throw new TypeError('`jest.setSystemTime()` is not available when using legacy fake timers.');
        }
      },
      setTimeout,
      setTimerTickMode: mode => {
        const fakeTimers = _getFakeTimers();
        if (fakeTimers === this.environment.fakeTimersModern) {
          fakeTimers.setTimerTickMode(mode);
        } else {
          throw new TypeError('`jest.setTimerTickMode()` is not available when using legacy fake timers.');
        }
        return jestObject;
      },
      spyOn,
      unmock,
      unstable_mockModule: mockModule,
      unstable_unmockModule: unmockModule,
      useFakeTimers,
      useRealTimers
    };
    return jestObject;
  }
}
exports.JestGlobals = JestGlobals;

/***/ },

/***/ "./src/internals/MockState.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.MockState = void 0;
exports.generateMock = generateMock;
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

const NODE_MODULES = `${path().sep}node_modules${path().sep}`;
const unmockRegExpCache = new WeakMap();
const transitiveCacheKey = (from, moduleID) => `${from}\0${moduleID}`;
class MockState {
  resolution;
  unmockList;
  shouldAutoMock;
  explicitCjsMock = new Map();
  explicitEsmMock = new Map();
  cjsFactories = new Map();
  esmFactories = new Map();
  virtualCjsMocks = new Map();
  virtualEsmMocks = new Map();
  shouldMockCache = new Map();
  shouldUnmockTransitiveDepsCache = new Map();
  transitiveShouldMock = new Map();
  mockMetaDataCache = new Map();
  onGenerateMockCallbacks = new Set();
  constructor(resolution, config) {
    this.resolution = resolution;
    this.shouldAutoMock = config.automock;
    let unmock = unmockRegExpCache.get(config);
    if (!unmock && config.unmockedModulePathPatterns) {
      unmock = new RegExp(config.unmockedModulePathPatterns.join('|'));
      unmockRegExpCache.set(config, unmock);
    }
    this.unmockList = unmock;
  }
  shouldMockCjs(from, moduleName) {
    const moduleID = this.resolution.getCjsModuleId(this.virtualCjsMocks, from, moduleName);
    return {
      moduleID,
      shouldMock: this.decideSync(from, moduleName, moduleID, 'cjs')
    };
  }
  shouldMockEsmSync(from, moduleName) {
    const moduleID = this.resolution.getEsmModuleId(this.virtualEsmMocks, from, moduleName);
    return {
      moduleID,
      shouldMock: this.decideSync(from, moduleName, moduleID, 'esm')
    };
  }
  async shouldMockEsmAsync(from, moduleName) {
    const moduleID = await this.resolution.getEsmModuleIdAsync(this.virtualEsmMocks, from, moduleName);
    return {
      moduleID,
      shouldMock: await this.decideEsmAsync(from, moduleName, moduleID)
    };
  }
  async decideEsmAsync(from, moduleName, moduleID) {
    const explicit = this.explicitEsmMock.get(moduleID);
    if (explicit !== undefined) return explicit;
    const key = transitiveCacheKey(from, moduleID);
    if (!this.shouldAutoMock || this.resolution.isCoreModule(moduleName) || this.shouldUnmockTransitiveDepsCache.get(key)) {
      return false;
    }
    const cached = this.shouldMockCache.get(moduleID);
    if (cached !== undefined) return cached;
    let modulePath;
    try {
      modulePath = await this.resolution.resolveEsmAsync(from, moduleName);
    } catch (error) {
      const manualMock = await this.resolution.getEsmMockModuleAsync(from, moduleName);
      if (manualMock) {
        this.shouldMockCache.set(moduleID, true);
        return true;
      }
      throw error;
    }
    if (this.unmockList?.test(modulePath)) {
      this.shouldMockCache.set(moduleID, false);
      return false;
    }
    const currentModuleID = await this.resolution.getEsmModuleIdAsync(this.virtualEsmMocks, from);
    return this.applyTransitive(moduleID, currentModuleID, modulePath, from, key, this.explicitEsmMock);
  }
  decideSync(from, moduleName, moduleID, mode) {
    const explicitMap = mode === 'cjs' ? this.explicitCjsMock : this.explicitEsmMock;
    const explicit = explicitMap.get(moduleID);
    if (explicit !== undefined) return explicit;
    const key = transitiveCacheKey(from, moduleID);
    if (!this.shouldAutoMock || this.resolution.isCoreModule(moduleName) || this.shouldUnmockTransitiveDepsCache.get(key)) {
      return false;
    }
    const cached = this.shouldMockCache.get(moduleID);
    if (cached !== undefined) return cached;
    let modulePath;
    try {
      modulePath = mode === 'cjs' ? this.resolution.resolveCjs(from, moduleName) : this.resolution.resolveEsm(from, moduleName);
    } catch (error) {
      const manualMock = mode === 'cjs' ? this.resolution.getCjsMockModule(from, moduleName) : this.resolution.getEsmMockModule(from, moduleName);
      if (manualMock) {
        this.shouldMockCache.set(moduleID, true);
        return true;
      }
      throw error;
    }
    if (this.unmockList?.test(modulePath)) {
      this.shouldMockCache.set(moduleID, false);
      return false;
    }
    const currentModuleID = mode === 'cjs' ? this.resolution.getCjsModuleId(this.virtualCjsMocks, from) : this.resolution.getEsmModuleId(this.virtualEsmMocks, from);
    return this.applyTransitive(moduleID, currentModuleID, modulePath, from, key, explicitMap);
  }
  applyTransitive(moduleID, currentModuleID, modulePath, from, key, explicitMap) {
    if (this.transitiveShouldMock.get(currentModuleID) === false || from.includes(NODE_MODULES) && modulePath.includes(NODE_MODULES) && (this.unmockList && this.unmockList.test(from) || explicitMap.get(currentModuleID) === false)) {
      this.transitiveShouldMock.set(moduleID, false);
      this.shouldUnmockTransitiveDepsCache.set(key, true);
      return false;
    }
    this.shouldMockCache.set(moduleID, true);
    return true;
  }

  // ---- explicit registration ----

  setMock(from, moduleName, factory, options) {
    if (options?.virtual) {
      const mockPath = this.resolution.getModulePath(from, moduleName);
      this.virtualCjsMocks.set(mockPath, true);
    }
    const moduleID = this.resolution.getCjsModuleId(this.virtualCjsMocks, from, moduleName);
    this.explicitCjsMock.set(moduleID, true);
    this.cjsFactories.set(moduleID, factory);
  }
  setModuleMock(from, moduleName, factory, options) {
    if (options?.virtual) {
      const mockPath = this.resolution.getModulePath(from, moduleName);
      this.virtualEsmMocks.set(mockPath, true);
    }
    const moduleID = this.resolution.getEsmModuleId(this.virtualEsmMocks, from, moduleName);
    this.explicitEsmMock.set(moduleID, true);
    this.esmFactories.set(moduleID, factory);
  }
  disableAutomock() {
    this.shouldAutoMock = false;
  }
  enableAutomock() {
    this.shouldAutoMock = true;
  }
  unmockCjs(from, moduleName) {
    const moduleID = this.resolution.getCjsModuleId(this.virtualCjsMocks, from, moduleName);
    this.explicitCjsMock.set(moduleID, false);
  }
  unmockEsm(from, moduleName) {
    const moduleID = this.resolution.getEsmModuleId(this.virtualEsmMocks, from, moduleName);
    this.explicitEsmMock.set(moduleID, false);
  }
  deepUnmock(from, moduleName) {
    const moduleID = this.resolution.getCjsModuleId(this.virtualCjsMocks, from, moduleName);
    this.explicitCjsMock.set(moduleID, false);
    this.transitiveShouldMock.set(moduleID, false);
  }
  markExplicitCjsMock(from, moduleName) {
    const moduleID = this.resolution.getCjsModuleId(this.virtualCjsMocks, from, moduleName);
    this.explicitCjsMock.set(moduleID, true);
  }
  addOnGenerateMock(callback) {
    this.onGenerateMockCallbacks.add(callback);
  }
  getCjsModuleId(from, moduleName) {
    return this.resolution.getCjsModuleId(this.virtualCjsMocks, from, moduleName);
  }
  getEsmModuleId(from, moduleName) {
    return this.resolution.getEsmModuleId(this.virtualEsmMocks, from, moduleName);
  }
  getEsmModuleIdAsync(from, moduleName) {
    return this.resolution.getEsmModuleIdAsync(this.virtualEsmMocks, from, moduleName);
  }
  isExplicitlyUnmocked(moduleID) {
    return this.explicitCjsMock.get(moduleID) === false;
  }
  hasCjsFactory(moduleID) {
    return this.cjsFactories.has(moduleID);
  }
  getCjsFactory(moduleID) {
    return this.cjsFactories.get(moduleID);
  }
  hasEsmFactory(moduleID) {
    return this.esmFactories.has(moduleID);
  }
  getEsmFactory(moduleID) {
    return this.esmFactories.get(moduleID);
  }
  markTransitive(moduleID, value) {
    this.transitiveShouldMock.set(moduleID, value);
  }
  hasMockMetadata(modulePath) {
    return this.mockMetaDataCache.has(modulePath);
  }
  getMockMetadata(modulePath) {
    return this.mockMetaDataCache.get(modulePath);
  }
  setMockMetadata(modulePath, metadata) {
    this.mockMetaDataCache.set(modulePath, metadata);
  }
  notifyMockGenerated(moduleName, moduleMock) {
    let result = moduleMock;
    for (const callback of this.onGenerateMockCallbacks) {
      result = callback(moduleName, result);
    }
    return result;
  }

  // `resetModules` does not touch mock state - explicit mocks, factories, and
  // virtual marks survive a reset. Only `teardown` drops everything.
  clear() {
    this.cjsFactories.clear();
    this.esmFactories.clear();
    this.mockMetaDataCache.clear();
    this.shouldMockCache.clear();
    this.shouldUnmockTransitiveDepsCache.clear();
    this.explicitCjsMock.clear();
    this.explicitEsmMock.clear();
    this.transitiveShouldMock.clear();
    this.virtualCjsMocks.clear();
    this.virtualEsmMocks.clear();
    this.onGenerateMockCallbacks.clear();
  }
}
exports.MockState = MockState;
function generateMock(from, moduleName, options) {
  const {
    resolution,
    mockState,
    moduleMocker,
    registries,
    requireModule
  } = options;
  const modulePath = resolution.resolveCjsStub(from, moduleName) || resolution.resolveCjs(from, moduleName);
  if (!mockState.hasMockMetadata(modulePath)) {
    // This allows us to handle circular dependencies while generating an
    // automock
    mockState.setMockMetadata(modulePath, moduleMocker.getMetadata({}) || {});

    // In order to avoid it being possible for automocking to potentially
    // cause side-effects within the module environment, we need to execute
    // the module in isolation. This could cause issues if the module being
    // mocked has calls into side-effectful APIs on another module.
    const moduleExports = registries.withScratchRegistries(() => requireModule(from, moduleName));
    const mockMetadata = moduleMocker.getMetadata(moduleExports);
    if (mockMetadata == null) {
      throw new Error(`Failed to get mock metadata: ${modulePath}\n\n` + 'See: https://jestjs.io/docs/manual-mocks#content');
    }
    mockState.setMockMetadata(modulePath, mockMetadata);
  }
  const moduleMock = moduleMocker.generateFromMetadata(mockState.getMockMetadata(modulePath));
  return mockState.notifyMockGenerated(modulePath, moduleMock);
}

/***/ },

/***/ "./src/internals/ModuleExecutor.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.isCjsParseError = exports.ModuleExecutor = exports.CJS_PARSE_ERROR = void 0;
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
    return data;
  };
  return data;
}
function _transform() {
  const data = require("@jest/transform");
  _transform = function () {
    return data;
  };
  return data;
}
function _jestResolve() {
  const data = _interopRequireDefault(require("jest-resolve"));
  _jestResolve = function () {
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
var _nodeCapabilities = __webpack_require__("./src/internals/nodeCapabilities.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Marker used by the CJS-as-ESM SyntaxError fallback paths to distinguish
// parse-time errors (where retrying as ESM is correct) from runtime errors
// a user might throw from inside a module body.
const CJS_PARSE_ERROR = exports.CJS_PARSE_ERROR = Symbol('jest-runtime CJS parse error');
const isCjsParseError = error => (0, _jestUtil().isError)(error) && error[CJS_PARSE_ERROR] === true;
exports.isCjsParseError = isCjsParseError;
class ModuleExecutor {
  resolution;
  transformCache;
  environment;
  config;
  testPath;
  requireBuilder;
  testMainModule;
  jestGlobals;
  dynamicImport;
  currentlyExecutingManualMock = null;
  constructor(options) {
    this.resolution = options.resolution;
    this.transformCache = options.transformCache;
    this.environment = options.environment;
    this.config = options.config;
    this.testPath = options.testPath;
    this.requireBuilder = options.requireBuilder;
    this.testMainModule = options.testMainModule;
    this.jestGlobals = options.jestGlobals;
    this.dynamicImport = options.dynamicImport;
  }
  getCurrentlyExecutingManualMock() {
    return this.currentlyExecutingManualMock;
  }
  exec(localModule, options, moduleRegistry, from, moduleName) {
    if (!this.environment.global) {
      return 'env-disposed';
    }
    const module = localModule;
    const filename = module.filename;
    const origCurrExecutingManualMock = this.currentlyExecutingManualMock;
    this.currentlyExecutingManualMock = filename;
    try {
      module.children = [];
      Object.defineProperty(module, 'parent', {
        enumerable: true,
        get() {
          const key = from || '';
          return moduleRegistry.get(key) || null;
        }
      });
      const modulePaths = this.resolution.getModulePaths(module.path);
      const globalPaths = this.resolution.getGlobalPaths(moduleName);
      module.paths = [...modulePaths, ...globalPaths];
      Object.defineProperty(module, 'require', {
        value: this.requireBuilder.for(localModule, options)
      });
      const transformedCode = this.transformCache.transform(filename, options);
      const compiledFunction = this.compile(transformedCode, filename);
      if (compiledFunction === null) {
        return 'env-disposed';
      }
      const jestObject = this.jestGlobals.jestObjectFor(filename);
      const lastArgs = [this.config.injectGlobals ? jestObject : undefined, ...this.config.sandboxInjectedGlobals.map(globalVariable => {
        if (this.environment.global[globalVariable]) {
          return this.environment.global[globalVariable];
        }
        throw new Error(`You have requested '${globalVariable}' as a global variable, but it was not present. Please check your config or your global environment.`);
      })];
      if (!this.testMainModule.current && filename === this.testPath) {
        this.testMainModule.current = module;
      }
      Object.defineProperty(module, 'main', {
        enumerable: true,
        value: this.testMainModule.current
      });
      try {
        compiledFunction.call(module.exports, module,
        // module object
        module.exports,
        // module exports
        module.require,
        // require implementation
        module.path,
        // __dirname
        module.filename,
        // __filename
        lastArgs[0], ...lastArgs.slice(1).filter(_jestUtil().isNonNullable));
      } catch (error) {
        this.handleExecutionError(error, module);
      }
    } finally {
      this.currentlyExecutingManualMock = origCurrExecutingManualMock;
    }
    return 'loaded';
  }
  compile(scriptSource, filename) {
    const vmContext = this.environment.getVmContext();
    if (vmContext == null) {
      return null;
    }
    try {
      const scriptFilename = this.resolution.isCoreModule(filename) ? `jest-nodejs-core-${filename}` : filename;
      return (0, _nodeVm().compileFunction)(scriptSource, this.constructInjectedModuleParameters(), {
        filename: scriptFilename,
        importModuleDynamically: async (specifier, _function, importAttributes) => {
          (0, _jestUtil().invariant)(_nodeCapabilities.runtimeSupportsVmModules, 'You need to run with a version of node that supports ES Modules in the VM API. See https://jestjs.io/docs/ecmascript-modules');
          return this.dynamicImport(specifier, scriptFilename, vmContext, importAttributes);
        },
        parsingContext: vmContext
      });
    } catch (error) {
      // Tag so callers can distinguish parse-time SyntaxErrors (where the
      // ESM-syntax-in-CJS fallback applies) from runtime SyntaxErrors a user
      // might throw from inside a CJS module body.
      if ((0, _jestUtil().isError)(error)) {
        error[CJS_PARSE_ERROR] = true;
      }
      throw (0, _transform().handlePotentialSyntaxError)(error);
    }
  }
  constructInjectedModuleParameters() {
    return ['module', 'exports', 'require', '__dirname', '__filename', this.config.injectGlobals ? 'jest' : undefined, ...this.config.sandboxInjectedGlobals].filter(_jestUtil().isNonNullable);
  }
  handleExecutionError(error, module) {
    const moduleNotFoundError = _jestResolve().default.tryCastModuleNotFoundError(error);
    if (moduleNotFoundError) {
      if (!moduleNotFoundError.requireStack) {
        moduleNotFoundError.requireStack = [module.filename || module.id];
        for (let cursor = module.parent; cursor; cursor = cursor.parent) {
          moduleNotFoundError.requireStack.push(cursor.filename || cursor.id);
        }
        moduleNotFoundError.buildMessage(this.config.rootDir);
      }
      throw moduleNotFoundError;
    }
    throw error;
  }
}
exports.ModuleExecutor = ModuleExecutor;

/***/ },

/***/ "./src/internals/ModuleRegistries.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.ModuleRegistries = void 0;
function _nodeModule() {
  const data = _interopRequireDefault(require("node:module"));
  _nodeModule = function () {
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
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Only expose ESM entries whose `namespace` is readable without throwing or
// exposing TDZ values: `unlinked`/`linking` throw `ERR_VM_MODULE_STATUS`, and
// a `linked` SourceTextModule's namespace properties are in TDZ until
// evaluate runs (reading them throws `ReferenceError`).
const isLiveEsm = entry => {
  if (!entry || entry instanceof Promise) return false;
  const status = entry.status;
  return status === 'evaluated' || status === 'errored';
};
const notPermittedMethod = () => true;
class Isolation {
  cjs = new Map();
  esm = new Map();
  mock = new Map();
  clear() {
    this.cjs.clear();
    this.esm.clear();
    this.mock.clear();
  }
}
class ModuleRegistries {
  moduleRegistry = new Map();
  internalModuleRegistry = new Map();
  esModuleRegistry = new Map();
  mockRegistry = new Map();
  moduleMockRegistry = new Map();
  isolation = null;
  esmRequireCacheWrappers = new WeakMap();
  getCjs(modulePath) {
    return (this.isolation?.cjs ?? this.moduleRegistry).get(modulePath);
  }
  setCjs(modulePath, module) {
    (this.isolation?.cjs ?? this.moduleRegistry).set(modulePath, module);
  }
  hasCjs(modulePath) {
    return (this.isolation?.cjs ?? this.moduleRegistry).has(modulePath);
  }
  deleteCjs(modulePath) {
    (this.isolation?.cjs ?? this.moduleRegistry).delete(modulePath);
  }
  getInternalCjs(modulePath) {
    return this.internalModuleRegistry.get(modulePath);
  }
  setInternalCjs(modulePath, module) {
    this.internalModuleRegistry.set(modulePath, module);
  }
  hasInternalCjs(modulePath) {
    return this.internalModuleRegistry.has(modulePath);
  }
  getEsm(key) {
    return (this.isolation?.esm ?? this.esModuleRegistry).get(key);
  }
  setEsm(key, module) {
    (this.isolation?.esm ?? this.esModuleRegistry).set(key, module);
  }
  hasEsm(key) {
    return (this.isolation?.esm ?? this.esModuleRegistry).has(key);
  }

  // Reads cascade: isolated overlay first, fall back to main. Writes go to
  // the active overlay only. This lets `jest.isolateModules` inherit mock
  // instances the user set up outside (so `.mockImplementation(...)` on the
  // outer instance still applies to inner reads) while still allowing the
  // isolation block to install its own mocks that don't leak back out.
  getMock(moduleID) {
    const fromIsolated = this.isolation?.mock.get(moduleID);
    if (fromIsolated !== undefined) return fromIsolated;
    return this.mockRegistry.get(moduleID);
  }
  setMock(moduleID, module) {
    (this.isolation?.mock ?? this.mockRegistry).set(moduleID, module);
  }
  hasMock(moduleID) {
    return (this.isolation?.mock.has(moduleID) ?? false) || this.mockRegistry.has(moduleID);
  }
  getModuleMock(moduleID) {
    return this.moduleMockRegistry.get(moduleID);
  }
  setModuleMock(moduleID, module) {
    this.moduleMockRegistry.set(moduleID, module);
  }
  hasModuleMock(moduleID) {
    return this.moduleMockRegistry.has(moduleID);
  }
  getActiveEsmRegistry() {
    return this.isolation?.esm ?? this.esModuleRegistry;
  }
  getActiveCjsRegistry() {
    return this.isolation?.cjs ?? this.moduleRegistry;
  }
  getInternalCjsRegistry() {
    return this.internalModuleRegistry;
  }
  getActiveMockRegistry() {
    return this.isolation?.mock ?? this.mockRegistry;
  }
  isIsolated() {
    return this.isolation !== null;
  }
  enterIsolated(callerName) {
    if (this.isIsolated()) {
      const other = callerName === 'isolateModules' ? 'isolateModulesAsync' : 'isolateModules';
      throw new Error(`${callerName} cannot be nested inside another ${callerName} or ${other}.`);
    }
    this.isolation = new Isolation();
  }
  exitIsolated() {
    this.isolation?.clear();
    this.isolation = null;
  }

  // Loads `fn` against fresh CJS + mock registries, then restores the
  // originals. Used by `_generateMock` to keep automock loading from
  // polluting the real caches.
  withScratchRegistries(fn) {
    const originalMock = this.mockRegistry;
    const originalModule = this.moduleRegistry;
    this.mockRegistry = new Map();
    this.moduleRegistry = new Map();
    try {
      return fn();
    } finally {
      this.mockRegistry = originalMock;
      this.moduleRegistry = originalModule;
    }
  }
  wrapEsmForRequireCache(filename, esm) {
    const existing = this.esmRequireCacheWrappers.get(esm);
    if (existing) return existing;
    const dir = path().dirname(filename);
    const wrapper = {
      children: [],
      exports: esm.namespace,
      filename,
      id: filename,
      isPreloading: false,
      loaded: true,
      parent: null,
      path: dir,
      paths: _nodeModule().default.Module._nodeModulePaths(dir),
      require: () => {
        throw new Error('require() on a require.cache ESM entry is not supported');
      }
    };
    this.esmRequireCacheWrappers.set(esm, wrapper);
    return wrapper;
  }
  createRequireCacheProxy() {
    const esmEntry = key => {
      const entry = this.esModuleRegistry.get(key);
      if (!isLiveEsm(entry)) return undefined;
      return this.wrapEsmForRequireCache(key, entry);
    };
    return new Proxy(Object.create(null), {
      defineProperty: notPermittedMethod,
      deleteProperty: notPermittedMethod,
      get: (_target, key) => {
        if (typeof key !== 'string') return undefined;
        return this.moduleRegistry.get(key) ?? esmEntry(key);
      },
      getOwnPropertyDescriptor() {
        return {
          configurable: true,
          enumerable: true
        };
      },
      has: (_target, key) => {
        if (typeof key !== 'string') return false;
        return this.moduleRegistry.has(key) || isLiveEsm(this.esModuleRegistry.get(key));
      },
      ownKeys: () => {
        const keys = new Set(this.moduleRegistry.keys());
        for (const [key, entry] of this.esModuleRegistry) {
          if (isLiveEsm(entry)) keys.add(key);
        }
        return [...keys];
      },
      set: notPermittedMethod
    });
  }
  clearForReset() {
    this.exitIsolated();
    this.mockRegistry.clear();
    this.moduleRegistry.clear();
    this.esModuleRegistry.clear();
    this.moduleMockRegistry.clear();
  }
  clear() {
    this.clearForReset();
    this.internalModuleRegistry.clear();
  }
}
exports.ModuleRegistries = ModuleRegistries;

/***/ },

/***/ "./src/internals/Resolution.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.isWasm = exports.Resolution = void 0;
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
function _jestResolve() {
  const data = _interopRequireDefault(require("jest-resolve"));
  _jestResolve = function () {
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
 */

const isWasm = modulePath => modulePath.endsWith('.wasm');
exports.isWasm = isWasm;
class Resolution {
  resolver;
  cjsConditions;
  esmConditions;
  extensionsToTreatAsEsm;
  cjsCache = new Map();
  esmCache = new Map();
  manualMockCache = new Map();
  constructor(resolver, envExportConditions, extensionsToTreatAsEsm) {
    this.resolver = resolver;
    this.cjsConditions = [...new Set(['require', 'node', 'default', ...envExportConditions])];
    this.esmConditions = [...new Set(['import', 'default', ...envExportConditions])];
    this.extensionsToTreatAsEsm = extensionsToTreatAsEsm;
  }
  shouldLoadAsEsm(modulePath) {
    return isWasm(modulePath) || _jestResolve().default.unstable_shouldLoadAsEsm(modulePath, this.extensionsToTreatAsEsm);
  }
  resolveCjs(from, to) {
    if (!to) return from;
    return this.resolveCached(from, to, this.cjsCache, this.cjsConditions);
  }
  resolveEsm(from, to) {
    if (!to) return from;
    return this.resolveCached(from, to, this.esmCache, this.esmConditions);
  }
  resolveEsmAsync(from, to) {
    if (!to) return Promise.resolve(from);
    return this.resolver.resolveModuleAsync(from, to, {
      conditions: this.esmConditions
    });
  }
  getCjsModuleId(virtualMocks, from, moduleName) {
    return this.resolver.getModuleID(virtualMocks, from, moduleName, {
      conditions: this.cjsConditions
    });
  }
  getEsmModuleId(virtualMocks, from, moduleName) {
    return this.resolver.getModuleID(virtualMocks, from, moduleName, {
      conditions: this.esmConditions
    });
  }
  getEsmModuleIdAsync(virtualMocks, from, moduleName) {
    return this.resolver.getModuleIDAsync(virtualMocks, from, moduleName, {
      conditions: this.esmConditions
    });
  }
  getCjsMockModule(from, moduleName) {
    return this.resolver.getMockModule(from, moduleName, {
      conditions: this.cjsConditions
    });
  }
  getEsmMockModule(from, moduleName) {
    return this.resolver.getMockModule(from, moduleName, {
      conditions: this.esmConditions
    });
  }
  getEsmMockModuleAsync(from, moduleName) {
    return this.resolver.getMockModuleAsync(from, moduleName, {
      conditions: this.esmConditions
    });
  }

  // Resolves the manual mock module path from a (potentially aliased) module
  // name. Covers three shapes:
  //
  // A. Core module specifier i.e. ['fs', 'node:fs']:
  //    Normalize then check for a root manual mock '<rootDir>/__mocks__/'.
  //
  // B. Node module specifier i.e. ['jest', 'react']:
  //    Look for root manual mock.
  //
  // C. Relative/Absolute path:
  //    If the actual module file has a __mocks__ dir sitting immediately next
  //    to it, look to see if there is a manual mock for this file.
  //
  //      subDir1/my_module.js
  //      subDir1/__mocks__/my_module.js
  //      subDir2/my_module.js
  //      subDir2/__mocks__/my_module.js
  //
  //    Where some other module does a relative require into each of the
  //    respective subDir{1,2} directories and expects a manual mock
  //    corresponding to that particular my_module.js file.
  findManualMock(from, moduleName) {
    // Normalize core specifiers (`node:fs` → `fs`) before building the cache
    // key so the two forms share an entry instead of populating two.
    const canonicalName = this.isCoreModule(moduleName) ? this.normalizeCoreModuleSpecifier(moduleName) : moduleName;
    const cacheKey = `${from}\0${canonicalName}`;
    let result = this.manualMockCache.get(cacheKey);
    if (result === undefined) {
      result = this.computeManualMock(from, canonicalName);
      this.manualMockCache.set(cacheKey, result);
    }
    return result;
  }
  computeManualMock(from, moduleName) {
    if (this.isCoreModule(moduleName)) {
      return this.getCjsMockModule(from, moduleName);
    }
    const rootMock = this.getCjsMockModule(from, moduleName);
    if (rootMock) return rootMock;
    const modulePath = this.resolveCjs(from, moduleName);
    const sibling = path().join(path().dirname(modulePath), '__mocks__', path().basename(modulePath));
    return fs().existsSync(sibling) ? sibling : null;
  }
  resolveCjsStub(from, moduleName) {
    return this.resolver.resolveStubModuleName(from, moduleName, {
      conditions: this.cjsConditions
    });
  }
  getModulePaths(from) {
    return this.resolver.getModulePaths(from);
  }
  getGlobalPaths(moduleName) {
    return this.resolver.getGlobalPaths(moduleName);
  }
  isCoreModule(name) {
    return this.resolver.isCoreModule(name);
  }
  normalizeCoreModuleSpecifier(name) {
    return this.resolver.normalizeCoreModuleSpecifier(name);
  }
  getModule(name) {
    return this.resolver.getModule(name);
  }
  getModulePath(from, moduleName) {
    return this.resolver.getModulePath(from, moduleName);
  }
  canResolveSync() {
    return this.resolver.canResolveSync();
  }
  resolveCjsFromDirIfExists(dir, name, paths) {
    return this.resolver.resolveModuleFromDirIfExists(dir, name, {
      conditions: this.cjsConditions,
      paths
    });
  }
  clear() {
    this.cjsCache.clear();
    this.esmCache.clear();
    this.manualMockCache.clear();
  }
  resolveCached(from, to, cache, conditions) {
    const key = `${from}\0${to}`;
    const cached = cache.get(key);
    if (cached !== undefined) return cached;
    const resolved = this.resolver.resolveModule(from, to, {
      conditions
    });
    cache.set(key, resolved);
    return resolved;
  }
}
exports.Resolution = Resolution;

/***/ },

/***/ "./src/internals/TestMainModule.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.TestMainModule = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Shared cell for `require.main`: the executor writes it when the test file
// loads, the require-builder reads it when attaching `module.require`.
class TestMainModule {
  current = null;
  reset() {
    this.current = null;
  }
}
exports.TestMainModule = TestMainModule;

/***/ },

/***/ "./src/internals/TestState.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.TestState = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class TestState {
  state = 'loading';
  logFormattedReferenceError;
  constructor(logFormattedReferenceError) {
    this.logFormattedReferenceError = logFormattedReferenceError;
  }
  isTornDown() {
    return this.state === 'tornDown';
  }
  isBetweenTests() {
    return this.state === 'betweenTests';
  }

  /**
   * Logs a post-teardown reference error and sets `process.exitCode = 1` if
   * the runtime has been torn down. Returns `true` if the caller should bail
   * out (i.e. it was torn down), `false` otherwise.
   */
  bailIfTornDown(message) {
    if (this.state !== 'tornDown') return false;
    this.logFormattedReferenceError(message);
    process.exitCode = 1;
    return true;
  }

  /**
   * Like {@link bailIfTornDown}, but throws a `ReferenceError` with the same
   * `message` instead of returning a flag. Use at call sites that can't bail
   * with `return` (e.g. inside an `async` function whose return type does not
   * allow `void`/`null`).
   */
  throwIfTornDown(message) {
    if (this.bailIfTornDown(message)) {
      throw new ReferenceError(message);
    }
  }
  throwIfBetweenTests(message) {
    if (this.state === 'betweenTests') {
      throw new ReferenceError(message);
    }
  }
  enterTestCode() {
    this.state = 'inTest';
  }
  leaveTestCode() {
    this.state = 'betweenTests';
  }
  teardown() {
    this.state = 'tornDown';
  }
}
exports.TestState = TestState;

/***/ },

/***/ "./src/internals/TransformCache.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.TransformCache = void 0;
function _stripBom() {
  const data = _interopRequireDefault(require("strip-bom"));
  _stripBom = function () {
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

class TransformCache {
  scriptTransformer;
  fileCache;
  getFullTransformationOptions;
  transforms = new Map();
  mutex = new Map();
  sourceMaps = new Map();
  constructor(scriptTransformer, fileCache, getFullTransformationOptions) {
    this.scriptTransformer = scriptTransformer;
    this.fileCache = fileCache;
    this.getFullTransformationOptions = getFullTransformationOptions;
  }
  transform(filename, options) {
    const source = this.fileCache.readFile(filename);
    if (options?.isInternalModule) return source;
    const transformedFile = this.scriptTransformer.transform(filename, this.getFullTransformationOptions(options), source);
    this.transforms.set(filename, transformedFile);
    if (transformedFile.sourceMapPath) {
      this.sourceMaps.set(filename, transformedFile.sourceMapPath);
    }
    return transformedFile.code;
  }
  async transformAsync(filename, options) {
    const source = this.fileCache.readFile(filename);
    if (options?.isInternalModule) return source;
    const transformedFile = await this.scriptTransformer.transformAsync(filename, this.getFullTransformationOptions(options), source);
    if (this.transforms.get(filename)?.code !== transformedFile.code) {
      this.transforms.set(filename, transformedFile);
    }
    if (transformedFile.sourceMapPath) {
      this.sourceMaps.set(filename, transformedFile.sourceMapPath);
    }
    return transformedFile.code;
  }
  canTransformSync(filename) {
    return this.scriptTransformer.canTransformSync(filename);
  }
  getCachedSource(filename) {
    return this.transforms.get(filename)?.code;
  }

  // Reads + transforms a `.json` file's source, returning the transformed
  // text (still a string). Caller is responsible for `JSON.parse`-ing in the
  // appropriate realm.
  transformJson(filename, options) {
    const source = (0, _stripBom().default)(this.fileCache.readFile(filename));
    return this.scriptTransformer.transformJson(filename, this.getFullTransformationOptions(options), source);
  }
  getEntries() {
    return this.transforms;
  }
  getSourceMaps() {
    return this.sourceMaps;
  }

  // Mutex deduplicates parallel `transformAsync`s of the same module across
  // concurrent `loadEsmModule` calls on the legacy async path. Goes away
  // once min-Node ≥ v24.9 makes that path obsolete.
  hasMutex(key) {
    return this.mutex.has(key);
  }
  awaitMutex(key) {
    return this.mutex.get(key);
  }
  setMutex(key, promise) {
    this.mutex.set(key, promise);
  }
  clearMutex(key) {
    this.mutex.delete(key);
  }

  // `resetModules` calls this; source maps are preserved so post-reset stack
  // traces still resolve. `teardown` calls `clear()` instead.
  clearForReset() {
    this.transforms.clear();
    this.mutex.clear();
  }
  clear() {
    this.clearForReset();
    this.sourceMaps.clear();
  }
}
exports.TransformCache = TransformCache;

/***/ },

/***/ "./src/internals/V8CoverageCollector.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.V8CoverageCollector = void 0;
function _nodeUrl() {
  const data = require("node:url");
  _nodeUrl = function () {
    return data;
  };
  return data;
}
function _collectV8Coverage() {
  const data = require("collect-v8-coverage");
  _collectV8Coverage = function () {
    return data;
  };
  return data;
}
function _transform() {
  const data = require("@jest/transform");
  _transform = function () {
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

class V8CoverageCollector {
  coverageOptions;
  config;
  transformCache;
  instrumenter;
  result;
  sources;
  constructor(coverageOptions, config, transformCache) {
    this.coverageOptions = coverageOptions;
    this.config = config;
    this.transformCache = transformCache;
  }
  async start() {
    this.instrumenter = new (_collectV8Coverage().CoverageInstrumenter)();
    this.sources = new Map();
    await this.instrumenter.startInstrumenting();
  }
  async stop() {
    if (!this.instrumenter || !this.sources) {
      throw new Error('You need to call `collectV8Coverage` first.');
    }
    this.result = await this.instrumenter.stopInstrumenting();
    this.sources = new Map([...this.sources, ...this.transformCache.getEntries()]);
  }

  // Snapshot transforms about to be cleared (e.g. by `resetModules`) so the
  // mapping from URL to transformed source survives across the reset.
  snapshotTransforms() {
    if (!this.coverageOptions.collectCoverage || this.coverageOptions.coverageProvider !== 'v8' || !this.sources) {
      return;
    }
    this.sources = new Map([...this.sources, ...this.transformCache.getEntries()]);
  }
  getResult() {
    if (!this.result || !this.sources) {
      throw new Error('You need to call `stopCollectingV8Coverage` first.');
    }
    const sources = this.sources;
    return this.result.filter(res => res.url.startsWith('file://')).map(res => ({
      ...res,
      url: (0, _nodeUrl().fileURLToPath)(res.url)
    })).filter(res =>
    // TODO: will this work on windows? It might be better if `shouldInstrument` deals with it anyways
    res.url.startsWith(this.coverageOptions.globalRootDir ?? this.config.rootDir) && (0, _transform().shouldInstrument)(res.url, this.coverageOptions, this.config, /* loadedFilenames */[...sources.keys()])).map(result => ({
      codeTransformResult: sources.get(result.url),
      result
    }));
  }
  reset() {
    this.sources?.clear();
    this.result = [];
    this.instrumenter = undefined;
  }
}
exports.V8CoverageCollector = V8CoverageCollector;

/***/ },

/***/ "./src/internals/cjsRequire.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.RequireBuilder = exports.JEST_RESOLVE_OUTSIDE_VM_OPTION = exports.CoreModuleProvider = void 0;
function _nodeModule() {
  const data = _interopRequireDefault(require("node:module"));
  _nodeModule = function () {
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
function _nodeUrl() {
  const data = require("node:url");
  _nodeUrl = function () {
    return data;
  };
  return data;
}
function _jestResolve() {
  const data = _interopRequireDefault(require("jest-resolve"));
  _jestResolve = function () {
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
var _helpers = __webpack_require__("./src/helpers.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const JEST_RESOLVE_OUTSIDE_VM_OPTION = exports.JEST_RESOLVE_OUTSIDE_VM_OPTION = Symbol.for('jest-resolve-outside-vm-option');
class RequireBuilder {
  resolution;
  registries;
  testMainModule;
  requireDispatch;
  requireInternal;
  constructor(options) {
    this.resolution = options.resolution;
    this.registries = options.registries;
    this.testMainModule = options.testMainModule;
    this.requireDispatch = options.requireDispatch;
    this.requireInternal = options.requireInternal;
  }
  for(from, options) {
    const resolveImpl = (moduleName, resolveOptions) => {
      const resolved = this.resolve(from.filename, moduleName, resolveOptions);
      if (resolveOptions?.[JEST_RESOLVE_OUTSIDE_VM_OPTION] && options?.isInternalModule) {
        return (0, _helpers.createOutsideJestVmPath)(resolved);
      }
      return resolved;
    };
    resolveImpl.paths = moduleName => this.resolvePaths(from.filename, moduleName);
    const moduleRequire = options?.isInternalModule ? moduleName => this.requireInternal(from.filename, moduleName) : moduleName => this.requireDispatch(from.filename, moduleName);
    moduleRequire.extensions = Object.create(null);
    moduleRequire.resolve = resolveImpl;
    moduleRequire.cache = this.registries.createRequireCacheProxy();
    Object.defineProperty(moduleRequire, 'main', {
      enumerable: true,
      value: this.testMainModule.current
    });
    return moduleRequire;
  }
  forFilename(filename) {
    return this.for({
      children: [],
      exports: {},
      filename,
      id: filename,
      isPreloading: false,
      loaded: false,
      path: path().dirname(filename)
    }, undefined);
  }
  resolve(from, moduleName, options = {}) {
    if (moduleName == null) {
      throw new Error('The first argument to require.resolve must be a string. Received null or undefined.');
    }
    if (path().isAbsolute(moduleName)) {
      const module = this.resolution.resolveCjsFromDirIfExists(moduleName, moduleName, []);
      if (module) {
        return module;
      }
    } else if (options.paths) {
      const module = this.resolution.resolveCjsStub(from, moduleName);
      if (module) {
        return module;
      }
      for (const searchPath of options.paths) {
        const absolutePath = path().resolve(from, '..', searchPath);

        // required to also resolve files without leading './' directly in the path
        const module = this.resolution.resolveCjsFromDirIfExists(absolutePath, moduleName, [absolutePath]);
        if (module) {
          return module;
        }
      }
      throw new (_jestResolve().default.ModuleNotFoundError)(`Cannot resolve module '${moduleName}' from paths ['${options.paths.join("', '")}'] from ${from}`);
    }
    try {
      return this.resolution.resolveCjs(from, moduleName);
    } catch (error) {
      const module = this.resolution.getCjsMockModule(from, moduleName);
      if (module) {
        return module;
      }
      throw error;
    }
  }
  resolvePaths(from, moduleName) {
    const fromDir = path().resolve(from, '..');
    if (moduleName == null) {
      throw new Error('The first argument to require.resolve.paths must be a string. Received null or undefined.');
    }
    if (moduleName.length === 0) {
      throw new Error('The first argument to require.resolve.paths must not be the empty string.');
    }
    if (moduleName[0] === '.') {
      return [fromDir];
    }
    if (this.resolution.isCoreModule(moduleName)) {
      return null;
    }
    const modulePaths = this.resolution.getModulePaths(fromDir);
    const globalPaths = this.resolution.getGlobalPaths(moduleName);
    return [...modulePaths, ...globalPaths];
  }
}
exports.RequireBuilder = RequireBuilder;
class CoreModuleProvider {
  mockedModuleClass;
  resolution;
  environment;
  requireBuilder;
  constructor(options) {
    this.resolution = options.resolution;
    this.environment = options.environment;
    this.requireBuilder = options.requireBuilder;
  }
  require(moduleName, supportPrefix) {
    const moduleWithoutNodePrefix = supportPrefix && this.resolution.normalizeCoreModuleSpecifier(moduleName);
    if (moduleWithoutNodePrefix === 'process') {
      return this.environment.global.process;
    }
    if (moduleWithoutNodePrefix === 'module') {
      return this.getMockedModuleClass();
    }
    const coreModule = require(moduleName);
    (0, _jestUtil().protectProperties)(coreModule);
    return coreModule;
  }
  getMockedModuleClass() {
    if (this.mockedModuleClass) {
      return this.mockedModuleClass;
    }
    const createRequire = modulePath => {
      const filename = typeof modulePath === 'string' ? modulePath.startsWith('file:///') ? (0, _nodeUrl().fileURLToPath)(new (_nodeUrl().URL)(modulePath)) : modulePath : (0, _nodeUrl().fileURLToPath)(modulePath);
      if (!path().isAbsolute(filename)) {
        const error = new TypeError(`The argument 'filename' must be a file URL object, file URL string, or absolute path string. Received '${filename}'`);
        error.code = 'ERR_INVALID_ARG_TYPE';
        throw error;
      }
      return this.requireBuilder.forFilename(filename);
    };
    class Module extends _nodeModule().default.Module {}
    for (const [key, value] of Object.entries(_nodeModule().default.Module)) {
      // @ts-expect-error: no index signature
      Module[key] = value;
    }
    Module.Module = Module;
    if ('createRequire' in _nodeModule().default) {
      Module.createRequire = createRequire;
    }
    if ('syncBuiltinESMExports' in _nodeModule().default) {
      // cast since TS seems very confused about whether it exists or not
      Module.syncBuiltinESMExports =
      // eslint-disable-next-line @typescript-eslint/no-empty-function
      function syncBuiltinESMExports() {};
    }
    this.mockedModuleClass = Module;
    return Module;
  }
  reset() {
    this.mockedModuleClass = undefined;
  }
}
exports.CoreModuleProvider = CoreModuleProvider;

/***/ },

/***/ "./src/internals/nodeCapabilities.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.supportsSyncEvaluate = exports.supportsNodeColonModulePrefixInRequire = exports.runtimeSupportsVmModules = void 0;
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
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

const runtimeSupportsVmModules = exports.runtimeSupportsVmModules = typeof _nodeVm().SyntheticModule === 'function';
const supportsSyncEvaluate = exports.supportsSyncEvaluate =
// @ts-expect-error - `hasAsyncGraph` is in Node v24.9+, not yet typed in @types/node@18
typeof _nodeVm().SourceTextModule?.prototype.hasAsyncGraph === 'function';
const supportsNodeColonModulePrefixInRequire = exports.supportsNodeColonModulePrefixInRequire = (() => {
  try {
    require('node:fs');
    return true;
  } catch {
    return false;
  }
})();

/***/ },

/***/ "./src/internals/syntheticBuilders.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.buildCjsAsEsmSyntheticModule = buildCjsAsEsmSyntheticModule;
exports.buildCoreSyntheticModule = buildCoreSyntheticModule;
exports.buildJsonSyntheticModule = buildJsonSyntheticModule;
exports.buildWasmSyntheticModule = buildWasmSyntheticModule;
exports.evaluateSyntheticModule = evaluateSyntheticModule;
exports.syntheticFromExports = syntheticFromExports;
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
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
var _helpers = __webpack_require__("./src/helpers.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Build a SyntheticModule from a plain exports record. The set of names and
// the value *references* are snapshotted at construction time, so later
// `exportsObject[k] = v` re-assignments or key add/delete won't leak into
// `evaluate()`. This is a shallow snapshot - mutating an exported object
// (`exportsObject.x.foo = ...`) is still observable through `setExport`.
function syntheticFromExports(identifier, context, exportsObject) {
  const entries = Object.entries(exportsObject);
  return new (_nodeVm().SyntheticModule)(entries.map(([key]) => key), function () {
    for (const [key, value] of entries) {
      this.setExport(key, value);
    }
  }, {
    context,
    identifier
  });
}
function buildJsonSyntheticModule(jsonText, identifier, context) {
  // JSON.parse runs in the body so a parse error surfaces during evaluate(),
  // matching Node's native JSON-module semantics.
  return new (_nodeVm().SyntheticModule)(['default'], function () {
    const obj = JSON.parse(jsonText);
    this.setExport('default', obj);
  }, {
    context,
    identifier
  });
}

// The body reads each import's namespace via `getDepNamespace`, which both the
// sync graph (closure over `scratch`) and the legacy path (closure over a
// pre-built `moduleLookup`) supply.
function buildWasmSyntheticModule(wasmModule, identifier, context, getDepNamespace) {
  const exports = WebAssembly.Module.exports(wasmModule);
  const imports = WebAssembly.Module.imports(wasmModule);
  return new (_nodeVm().SyntheticModule)(exports.map(({
    name
  }) => name), function () {
    const importsObject = {};
    for (const {
      module: depSpec,
      name
    } of imports) {
      if (!importsObject[depSpec]) {
        importsObject[depSpec] = {};
      }
      const namespace = getDepNamespace(depSpec);
      importsObject[depSpec][name] = namespace[name];
    }
    const wasmInstance = new WebAssembly.Instance(wasmModule, importsObject);
    for (const {
      name
    } of exports) {
      this.setExport(name, wasmInstance.exports[name]);
    }
  }, {
    context,
    identifier
  });
}
function buildCoreSyntheticModule(moduleName, context, requireCoreModule) {
  const required = requireCoreModule(moduleName, true);
  // should identifier be `node://${moduleName}`?
  return syntheticFromExports(moduleName, context, {
    ...required,
    default: required
  });
}

// Builds a SyntheticModule wrapping a CJS module's `module.exports` for
// import-from-ESM. Merges cjs-module-lexer's static export list with the
// runtime keys of the actual exports object (lexer can miss
// `Object.assign`-style patterns).
function buildCjsAsEsmSyntheticModule(from, modulePath, context, requireModuleOrMock, cjsExportsCache) {
  const cjs = requireModuleOrMock(from, modulePath);
  const parsedExports = cjsExportsCache.getExportsOf(from, modulePath);

  // CJS modules can legally set `module.exports` to `null` or a primitive.
  // Functions are also valid (e.g. `module.exports = fn; fn.helper = ...`).
  const cjsRecord = cjs !== null && (typeof cjs === 'object' || typeof cjs === 'function') ? cjs : null;
  const allCandidates = new Set([...parsedExports, ...(cjsRecord ? Object.keys(cjsRecord) : [])]);
  const cjsExports = [...allCandidates].filter(exportName => {
    // `default` is handled separately below as the whole module.exports.
    if (exportName === 'default' || cjsRecord == null) {
      return false;
    }
    return Object.hasOwn(cjsRecord, exportName);
  });
  return new (_nodeVm().SyntheticModule)([...cjsExports, 'default'], function () {
    if (cjsRecord != null) {
      for (const exportName of cjsExports) {
        this.setExport(exportName, Reflect.get(cjsRecord, exportName));
      }
    }
    // module.exports is the ESM default, matching Node's CJS-from-ESM behavior.
    // __esModule is not honored — see Node docs on named exports from CJS.
    this.setExport('default', cjs);
  }, {
    context,
    identifier: modulePath
  });
}

// On Node v22.21+ / v24.8+ a SyntheticModule starts in `'linked'` and the
// body runs synchronously even though `evaluate()` returns a Promise -
// return it sync so callers can store the actual module rather than a
// Promise that can poison the registry if microtask draining stalls. On
// older Node it starts `'unlinked'` and link/evaluate are genuinely async;
// fall back to the async path there (the async-only legacy ESM code paths
// handle the Promise return fine, and sync `require(esm)` doesn't exist on
// those versions anyway).
function evaluateSyntheticModule(module) {
  if (module.status === 'unlinked') {
    return evaluateSyntheticModuleAsync(module);
  }
  module.evaluate().catch(_helpers.noop);
  if (module.status === 'errored') {
    throw module.error;
  }
  (0, _jestUtil().invariant)(module.status === 'evaluated', `Synthetic module ${module.identifier} did not evaluate synchronously (status="${module.status}"). This is a bug in Jest, please report it!`);
  return module;
}
async function evaluateSyntheticModuleAsync(module) {
  await module.link(() => {
    throw new Error('This should never happen');
  });
  await module.evaluate();
  return module;
}

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
exports["default"] = void 0;
function _nodeModule() {
  const data = _interopRequireDefault(require("node:module"));
  _nodeModule = function () {
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
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
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
function _transform() {
  const data = require("@jest/transform");
  _transform = function () {
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
function _jestMessageUtil() {
  const data = require("jest-message-util");
  _jestMessageUtil = function () {
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
function _jestResolve() {
  const data = _interopRequireDefault(require("jest-resolve"));
  _jestResolve = function () {
    return data;
  };
  return data;
}
function _jestSnapshot() {
  const data = require("jest-snapshot");
  _jestSnapshot = function () {
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
var _helpers = __webpack_require__("./src/helpers.ts");
var _CjsExportsCache = __webpack_require__("./src/internals/CjsExportsCache.ts");
var _CjsLoader = __webpack_require__("./src/internals/CjsLoader.ts");
var _EsmLoader = __webpack_require__("./src/internals/EsmLoader.ts");
var _FileCache = __webpack_require__("./src/internals/FileCache.ts");
var _JestGlobals = __webpack_require__("./src/internals/JestGlobals.ts");
var _MockState = __webpack_require__("./src/internals/MockState.ts");
var _ModuleExecutor = __webpack_require__("./src/internals/ModuleExecutor.ts");
var _ModuleRegistries = __webpack_require__("./src/internals/ModuleRegistries.ts");
var _Resolution = __webpack_require__("./src/internals/Resolution.ts");
var _TestMainModule = __webpack_require__("./src/internals/TestMainModule.ts");
var _TestState = __webpack_require__("./src/internals/TestState.ts");
var _TransformCache = __webpack_require__("./src/internals/TransformCache.ts");
var _V8CoverageCollector = __webpack_require__("./src/internals/V8CoverageCollector.ts");
var _cjsRequire = __webpack_require__("./src/internals/cjsRequire.ts");
var _nodeCapabilities = __webpack_require__("./src/internals/nodeCapabilities.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Modules safe to require from the outside (not stateful, not prone to
// realm errors) and slow enough that paying the worker-cache hit is worth
// it. Internal context only - user `require()` from a test still goes
// through the VM.
const INTERNAL_MODULE_REQUIRE_OUTSIDE_OPTIMIZED_MODULES = new Set(['chalk']);

// Framework modules that define shared singleton state (e.g. `JestAssertionError`).
// Redirecting user requires to the internal registry ensures test code and the
// framework see the same class instances.
const FRAMEWORK_SINGLETON_MODULES = new Set(['@jest/expect', 'expect']);
const esmIsAvailable = typeof _nodeVm().SourceTextModule === 'function';
const defaultTransformOptions = {
  isInternalModule: false,
  supportsDynamicImport: esmIsAvailable,
  supportsExportNamespaceFrom: false,
  supportsStaticESM: false,
  supportsTopLevelAwait: false
};
const NODE_MODULES = `${path().sep}node_modules${path().sep}`;
const getModuleNameMapper = config => {
  if (Array.isArray(config.moduleNameMapper) && config.moduleNameMapper.length > 0) {
    return config.moduleNameMapper.map(([regex, moduleName]) => ({
      moduleName,
      regex: new RegExp(regex)
    }));
  }
  return null;
};
class Runtime {
  fileCache;
  _config;
  _coverageOptions;
  _environment;
  mockState;
  registries;
  testMainModule;
  requireBuilder;
  executor;
  esmLoader;
  cjsLoader;
  _moduleMocker;
  cjsExportsCache;
  _testPath;
  _resolution;
  transformCache;
  v8Coverage;
  coreModule;
  jestGlobals;
  testState;
  loggedReferenceErrors = new Set();
  constructor(config, environment, resolver, transformer, cacheFS, coverageOptions, testPath, globalConfig) {
    this.fileCache = new _FileCache.FileCache(cacheFS);
    this._config = config;
    this._coverageOptions = coverageOptions;
    this._environment = environment;
    this.registries = new _ModuleRegistries.ModuleRegistries();
    (0, _jestUtil().invariant)(this._environment.moduleMocker, '`moduleMocker` must be set on an environment when created');
    this._moduleMocker = this._environment.moduleMocker;
    this._testPath = testPath;
    this.testState = new _TestState.TestState(msg => this._logFormattedReferenceError(msg));
    this.transformCache = new _TransformCache.TransformCache(transformer, this.fileCache, options => this._getFullTransformationOptions(options));
    this.v8Coverage = new _V8CoverageCollector.V8CoverageCollector(coverageOptions, config, this.transformCache);
    this._resolution = new _Resolution.Resolution(resolver, this._environment.exportConditions?.() ?? [], config.extensionsToTreatAsEsm);
    this.mockState = new _MockState.MockState(this._resolution, config);
    this.cjsExportsCache = new _CjsExportsCache.CjsExportsCache({
      fileCache: this.fileCache,
      loadCoreReexport: (from, coreName) => this.requireModule(from, coreName),
      loadNativeAddon: (from, modulePath) => this.requireModuleOrMock(from, modulePath),
      resolution: this._resolution,
      transformCache: this.transformCache
    });
    // Construction is a DAG: testMainModule → requireBuilder → {coreModule,
    // executor} → cjsLoader. The two lambdas inside `requireBuilder`'s deps
    // close over `cjsLoader` (built last) and `this.requireModuleOrMock`,
    // but those callbacks aren't invoked until user code runs, so the
    // forward references are safe.
    this.testMainModule = new _TestMainModule.TestMainModule();
    this.requireBuilder = new _cjsRequire.RequireBuilder({
      registries: this.registries,
      requireDispatch: (from, moduleName) => this.requireModuleOrMock(from, moduleName),
      requireInternal: (from, moduleName) => this.requireInternalModule(from, moduleName),
      resolution: this._resolution,
      testMainModule: this.testMainModule
    });
    this.coreModule = new _cjsRequire.CoreModuleProvider({
      environment: this._environment,
      requireBuilder: this.requireBuilder,
      resolution: this._resolution
    });
    this.jestGlobals = new _JestGlobals.JestGlobals({
      clearAllMocks: () => this.clearAllMocks(),
      config,
      environment: this._environment,
      generateMock: (from, moduleName) => this._generateMock(from, moduleName),
      globalConfig,
      isolateModules: fn => this.isolateModules(fn),
      isolateModulesAsync: fn => this.isolateModulesAsync(fn),
      logFormattedReferenceError: msg => this._logFormattedReferenceError(msg),
      mockState: this.mockState,
      moduleMocker: this._moduleMocker,
      requireActual: (from, moduleName) => this.requireActual(from, moduleName),
      requireMock: (from, moduleName) => this.requireMock(from, moduleName),
      resetAllMocks: () => this.resetAllMocks(),
      resetModules: () => this.resetModules(),
      restoreAllMocks: () => this.restoreAllMocks(),
      setMock: (from, moduleName, mockFactory, options) => this.setMock(from, moduleName, mockFactory, options),
      setModuleMock: (from, moduleName, mockFactory, options) => this.setModuleMock(from, moduleName, mockFactory, options),
      testState: this.testState
    });
    this.esmLoader = new _EsmLoader.EsmLoader({
      cjsExportsCache: this.cjsExportsCache,
      coreModule: this.coreModule,
      environment: this._environment,
      fileCache: this.fileCache,
      jestGlobals: this.jestGlobals,
      mockState: this.mockState,
      registries: this.registries,
      requireModuleOrMock: (from, moduleName) => this.requireModuleOrMock(from, moduleName),
      resolution: this._resolution,
      shouldLoadAsEsm: modulePath => this.unstable_shouldLoadAsEsm(modulePath),
      testState: this.testState,
      transformCache: this.transformCache
    });
    this.executor = new _ModuleExecutor.ModuleExecutor({
      config,
      dynamicImport: (specifier, identifier, context, importAttributes) => this.esmLoader.dynamicImportFromCjs(specifier, identifier, context, importAttributes),
      environment: this._environment,
      jestGlobals: this.jestGlobals,
      requireBuilder: this.requireBuilder,
      resolution: this._resolution,
      testMainModule: this.testMainModule,
      testPath,
      transformCache: this.transformCache
    });
    this.cjsLoader = new _CjsLoader.CjsLoader({
      coreModule: this.coreModule,
      environment: this._environment,
      executor: this.executor,
      logFormattedReferenceError: msg => this._logFormattedReferenceError(msg),
      mockState: this.mockState,
      registries: this.registries,
      requireEsm: modulePath => this.esmLoader.requireEsmModule(modulePath),
      resolution: this._resolution,
      testState: this.testState,
      transformCache: this.transformCache
    });
    if (config.automock) {
      for (const filePath of config.setupFiles) {
        if (filePath.includes(NODE_MODULES)) {
          // shouldn't really matter, but in theory this will make sure the caching is correct
          const moduleID = this.unstable_shouldLoadAsEsm(filePath) ? this._resolution.getEsmModuleId(new Map(), filePath) : this._resolution.getCjsModuleId(new Map(), filePath);
          this.mockState.markTransitive(moduleID, false);
        }
      }
    }
    this.resetModules();
  }
  static shouldInstrument = _transform().shouldInstrument;
  static async createContext(config, options) {
    (0, _jestUtil().createDirectory)(config.cacheDirectory);
    const instance = await Runtime.createHasteMap(config, {
      console: options.console,
      maxWorkers: options.maxWorkers,
      resetCache: !config.cache,
      watch: options.watch,
      watchman: options.watchman
    });
    const hasteMap = await instance.build();
    return {
      config,
      hasteFS: hasteMap.hasteFS,
      moduleMap: hasteMap.moduleMap,
      resolver: Runtime.createResolver(config, hasteMap.moduleMap)
    };
  }
  static createHasteMap(config, options) {
    const ignorePatternParts = [...config.modulePathIgnorePatterns, ...(options && options.watch ? config.watchPathIgnorePatterns : []), config.cacheDirectory.startsWith(config.rootDir + path().sep) && config.cacheDirectory].filter(Boolean);
    const ignorePattern = ignorePatternParts.length > 0 ? new RegExp(ignorePatternParts.join('|')) : undefined;
    return _jestHasteMap().default.create({
      cacheDirectory: config.cacheDirectory,
      computeSha1: config.haste.computeSha1,
      console: options?.console,
      dependencyExtractor: config.dependencyExtractor,
      enableSymlinks: config.haste.enableSymlinks,
      extensions: [_jestSnapshot().EXTENSION, ...config.moduleFileExtensions],
      forceNodeFilesystemAPI: config.haste.forceNodeFilesystemAPI,
      hasteImplModulePath: config.haste.hasteImplModulePath,
      hasteMapModulePath: config.haste.hasteMapModulePath,
      id: config.id,
      ignorePattern,
      maxWorkers: options?.maxWorkers || 1,
      mocksPattern: (0, _jestRegexUtil().escapePathForRegex)(`${path().sep}__mocks__${path().sep}`),
      platforms: config.haste.platforms || ['ios', 'android'],
      resetCache: options?.resetCache,
      retainAllFiles: config.haste.retainAllFiles || false,
      rootDir: config.rootDir,
      roots: config.roots,
      throwOnModuleCollision: config.haste.throwOnModuleCollision,
      useWatchman: options?.watchman,
      watch: options?.watch,
      workerThreads: options?.workerThreads
    });
  }
  static createResolver(config, moduleMap) {
    return new (_jestResolve().default)(moduleMap, {
      defaultPlatform: config.haste.defaultPlatform,
      extensions: config.moduleFileExtensions.map(extension => `.${extension}`),
      hasCoreModules: true,
      moduleDirectories: config.moduleDirectories,
      moduleNameMapper: getModuleNameMapper(config),
      modulePaths: config.modulePaths,
      platforms: config.haste.platforms,
      resolver: config.resolver,
      rootDir: config.rootDir
    });
  }

  // unstable as it should be replaced by https://github.com/nodejs/modules/issues/393, and we don't want people to use it
  unstable_shouldLoadAsEsm(modulePath) {
    return this._resolution.shouldLoadAsEsm(modulePath);
  }
  async unstable_importModule(from, moduleName) {
    return this.esmLoader.loadAndEvaluate(from, moduleName);
  }
  requireModule(from, moduleName, options, isRequireActual = false) {
    return this.cjsLoader.requireModule(from, moduleName, options, isRequireActual);
  }
  requireInternalModule(from, to) {
    if (to) {
      const require = _nodeModule().default.createRequire(from);
      if (INTERNAL_MODULE_REQUIRE_OUTSIDE_OPTIMIZED_MODULES.has(to)) {
        return require(to);
      }
      const outsideJestVmPath = (0, _helpers.decodePossibleOutsideJestVmPath)(to);
      if (outsideJestVmPath) {
        return require(outsideJestVmPath);
      }
    }
    return this.requireModule(from, to, {
      isInternalModule: true,
      supportsDynamicImport: _nodeCapabilities.runtimeSupportsVmModules,
      supportsExportNamespaceFrom: false,
      supportsStaticESM: false,
      supportsTopLevelAwait: false
    });
  }
  requireActual(from, moduleName) {
    if (FRAMEWORK_SINGLETON_MODULES.has(moduleName)) {
      return this.requireInternalModule(from, moduleName);
    }
    return this.requireModule(from, moduleName, undefined, true);
  }
  requireMock(from, moduleName) {
    return this._requireMockWithId(from, moduleName, this.mockState.getCjsModuleId(from, moduleName));
  }
  _requireMockWithId(from, moduleName, moduleID) {
    if (this.registries.hasMock(moduleID)) {
      return this.registries.getMock(moduleID);
    }
    const mockRegistry = this.registries.getActiveMockRegistry();
    const factory = this.mockState.getCjsFactory(moduleID);
    if (factory) {
      const module = factory();
      mockRegistry.set(moduleID, module);
      return module;
    }
    const manualMockPath = this._resolution.findManualMock(from, moduleName);
    if (manualMockPath) {
      const localModule = {
        children: [],
        exports: {},
        filename: manualMockPath,
        id: manualMockPath,
        isPreloading: false,
        loaded: false,
        path: path().dirname(manualMockPath)
      };
      this.cjsLoader.loadModule(localModule, from, moduleName, manualMockPath, undefined, mockRegistry);
      mockRegistry.set(moduleID, localModule.exports);
    } else {
      // Look for a real module to generate an automock from
      mockRegistry.set(moduleID, this._generateMock(from, moduleName));
    }
    return mockRegistry.get(moduleID);
  }
  _getFullTransformationOptions(options = defaultTransformOptions) {
    return {
      ...options,
      ...this._coverageOptions
    };
  }
  requireModuleOrMock(from, moduleName) {
    if (this.testState.bailIfTornDown('You are trying to `require` a file after the Jest environment has been torn down.')) {
      // @ts-expect-error: exiting early
      return;
    }

    // this module is unmockable
    if (moduleName === '@jest/globals') {
      // @ts-expect-error: we don't care that it's not assignable to T
      return this.jestGlobals.cjsGlobals(from);
    }
    try {
      const {
        shouldMock,
        moduleID
      } = this.mockState.shouldMockCjs(from, moduleName);
      if (shouldMock) {
        return this._requireMockWithId(from, moduleName, moduleID);
      }
      if (FRAMEWORK_SINGLETON_MODULES.has(moduleName)) {
        return this.requireInternalModule(from, moduleName);
      }
      return this.requireModule(from, moduleName);
    } catch (error) {
      const moduleNotFound = _jestResolve().default.tryCastModuleNotFoundError(error);
      if (moduleNotFound) {
        if (moduleNotFound.siblingWithSimilarExtensionFound === null || moduleNotFound.siblingWithSimilarExtensionFound === undefined) {
          moduleNotFound.hint = (0, _helpers.findSiblingsWithFileExtension)(this._config.moduleFileExtensions, from, moduleNotFound.moduleName || moduleName);
          moduleNotFound.siblingWithSimilarExtensionFound = Boolean(moduleNotFound.hint);
        }
        moduleNotFound.buildMessage(this._config.rootDir);
        throw moduleNotFound;
      }
      throw error;
    }
  }
  isolateModules(fn) {
    this.registries.enterIsolated('isolateModules');
    try {
      fn();
    } finally {
      this.registries.exitIsolated();
    }
  }
  async isolateModulesAsync(fn) {
    this.registries.enterIsolated('isolateModulesAsync');
    try {
      await fn();
    } finally {
      this.registries.exitIsolated();
    }
  }
  resetModules() {
    this.registries.clearForReset();
    this.cjsExportsCache.clear();
    this.fileCache.clear();
    this._resolution.clear();
    this.v8Coverage.snapshotTransforms();
    this.transformCache.clearForReset();
    if (this._environment) {
      if (this._environment.global) {
        this._moduleMocker.clearMocksOnScope(this._environment.global);
      }
      if (this._environment.fakeTimers) {
        this._environment.fakeTimers.clearAllTimers();
      }
    }
  }
  collectV8Coverage() {
    return this.v8Coverage.start();
  }
  stopCollectingV8Coverage() {
    return this.v8Coverage.stop();
  }
  getAllCoverageInfoCopy() {
    return (0, _jestUtil().deepCyclicCopy)(this._environment.global.__coverage__);
  }
  getAllV8CoverageInfoCopy() {
    return this.v8Coverage.getResult();
  }
  getSourceMaps() {
    return this.transformCache.getSourceMaps();
  }
  setMock(from, moduleName, mockFactory, options) {
    this.mockState.setMock(from, moduleName, mockFactory, options);
  }
  setModuleMock(from, moduleName, mockFactory, options) {
    this.mockState.setModuleMock(from, moduleName, mockFactory, options);
  }
  restoreAllMocks() {
    this._moduleMocker.restoreAllMocks();
  }
  resetAllMocks() {
    this._moduleMocker.resetAllMocks();
  }
  clearAllMocks() {
    this._moduleMocker.clearAllMocks();
  }
  enterTestCode() {
    this.testState.enterTestCode();
  }
  leaveTestCode() {
    this.testState.leaveTestCode();
  }
  teardown() {
    this.restoreAllMocks();
    this.resetModules();
    this.registries.clear();
    this.testMainModule.reset();
    this.mockState.clear();
    this.fileCache.clear();
    this.transformCache.clear();
    this.jestGlobals.clearJestObjectCache();
    this.v8Coverage.reset();
    this.coreModule.reset();
    this.loggedReferenceErrors.clear();
    this.testState.teardown();
  }
  _generateMock(from, moduleName) {
    return (0, _MockState.generateMock)(from, moduleName, {
      mockState: this.mockState,
      moduleMocker: this._moduleMocker,
      registries: this.registries,
      requireModule: (from, moduleName) => this.requireModule(from, moduleName),
      resolution: this._resolution
    });
  }
  _logFormattedReferenceError(errorMessage) {
    const testPath = this._testPath ? ` From ${(0, _slash().default)(path().relative(this._config.rootDir, this._testPath))}.` : '';
    const originalStack = new ReferenceError(`${errorMessage}${testPath}`).stack.split('\n')
    // Remove this file from the stack (jest-message-utils will keep one line)
    .filter(line => !line.includes(__filename)).join('\n');
    const {
      message,
      stack
    } = (0, _jestMessageUtil().separateMessageFromStack)(originalStack);
    const stackTrace = (0, _jestMessageUtil().formatStackTrace)(stack, this._config, {
      noStackTrace: false
    });
    const formattedMessage = `\n${message}${stackTrace ? `\n${stackTrace}` : ''}`;
    if (!this.loggedReferenceErrors.has(formattedMessage)) {
      console.error(formattedMessage);
      this.loggedReferenceErrors.add(formattedMessage);
    }
  }
  setGlobalsForRuntime(globals) {
    this.jestGlobals.setEnvGlobalsOverride(globals);
  }
}
exports["default"] = Runtime;
})();

module.exports = __webpack_exports__;
/******/ })()
;