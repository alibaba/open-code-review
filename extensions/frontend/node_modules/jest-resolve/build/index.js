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

/***/ "./src/ModuleNotFoundError.ts"
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

class ModuleNotFoundError extends Error {
  code = 'MODULE_NOT_FOUND';
  hint;
  requireStack;
  siblingWithSimilarExtensionFound;
  moduleName;
  _originalMessage;
  constructor(message, moduleName) {
    super(message);
    this._originalMessage = message;
    this.moduleName = moduleName;
  }
  buildMessage(rootDir) {
    if (!this._originalMessage) {
      this._originalMessage = this.message || '';
    }
    let message = this._originalMessage;
    if (this.requireStack?.length && this.requireStack.length > 1) {
      message += `

Require stack:
  ${this.requireStack.map(p => p.replace(`${rootDir}${path().sep}`, '')).map(_slash().default).join('\n  ')}
`;
    }
    if (this.hint) {
      message += this.hint;
    }
    this.message = message;
  }
  static duckType(error) {
    error.buildMessage = ModuleNotFoundError.prototype.buildMessage;
    return error;
  }
}
exports["default"] = ModuleNotFoundError;

/***/ },

/***/ "./src/defaultResolver.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.defaultResolver = exports.defaultAsyncResolver = exports["default"] = void 0;
function _nodeModule() {
  const data = require("node:module");
  _nodeModule = function () {
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
function _jestPnpResolver() {
  const data = _interopRequireDefault(require("jest-pnp-resolver"));
  _jestPnpResolver = function () {
    return data;
  };
  return data;
}
function _unrsResolver() {
  const data = require("unrs-resolver");
  _unrsResolver = function () {
    return data;
  };
  return data;
}
var _fileWalkers = __webpack_require__("./src/fileWalkers.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const handleResolveResult = result => {
  if (result.error) {
    throw new Error(result.error);
  }
  return result.path;
};
function baseResolver(path, options, async) {
  // https://github.com/oxc-project/oxc-resolver/issues/565
  // https://github.com/jestjs/jest/issues/15676
  if ((0, _nodeModule().isBuiltin)(path)) {
    return path;
  }
  if (process.versions.pnp && options.allowPnp !== false) {
    return (0, _jestPnpResolver().default)(path, {
      ...options,
      extensions: options.extensions
    });
  }
  if (path.startsWith('file://')) {
    path = (0, _nodeUrl().fileURLToPath)(path);
  }

  /* eslint-disable prefer-const */
  let {
    basedir,
    conditions,
    conditionNames,
    extensions,
    modules,
    moduleDirectory,
    paths,
    roots,
    rootDir,
    ...rest
    /* eslint-enable prefer-const */
  } = options;
  modules = modules || moduleDirectory;
  const resolveOptions = {
    conditionNames: conditionNames || conditions || ['require', 'node', 'default'],
    extensions: extensions,
    modules,
    roots: roots || (rootDir ? [rootDir] : undefined),
    ...rest
  };
  let unrsResolver = (0, _fileWalkers.getResolver)();
  if (unrsResolver) {
    unrsResolver = unrsResolver.cloneWithOptions(resolveOptions);
  } else {
    unrsResolver = new (_unrsResolver().ResolverFactory)(resolveOptions);
  }
  (0, _fileWalkers.setResolver)(unrsResolver);
  const finalResolver = resolve => {
    const resolveWithPathsFallback = result => {
      if (!result.path && paths?.length) {
        const modulesArr = modules == null || Array.isArray(modules) ? modules : [modules];
        if (modulesArr?.length) {
          paths = paths.filter(p => !modulesArr.includes(p));
        }
        if (paths.length > 0) {
          unrsResolver = unrsResolver.cloneWithOptions({
            ...resolveOptions,
            modules: paths
          });
          (0, _fileWalkers.setResolver)(unrsResolver);
          return resolve();
        }
      }
      return result;
    };
    const result = resolve();
    if ('then' in result) {
      return result.then(resolveWithPathsFallback).then(handleResolveResult);
    }
    return handleResolveResult(resolveWithPathsFallback(result));
  };
  return finalResolver(() => async ? unrsResolver.async(basedir, path) : unrsResolver.sync(basedir, path));
}
const defaultResolver = exports.defaultResolver = baseResolver;
const defaultAsyncResolver = (path, options) => baseResolver(path, options, true);
exports.defaultAsyncResolver = defaultAsyncResolver;
var _default = exports["default"] = defaultResolver;

/***/ },

/***/ "./src/fileWalkers.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.clearFsCache = clearFsCache;
exports.findClosestPackageJson = findClosestPackageJson;
exports.getResolver = getResolver;
exports.isDirectory = isDirectory;
exports.isFile = isFile;
exports.readPackageCached = readPackageCached;
exports.realpathSync = realpathSync;
exports.setResolver = setResolver;
function _nodePath() {
  const data = require("node:path");
  _nodePath = function () {
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

let unrsResolver;
function getResolver() {
  return unrsResolver;
}
function setResolver(nextResolver) {
  unrsResolver = nextResolver;
}
function clearFsCache() {
  unrsResolver?.clearCache();
  checkedPaths.clear();
  checkedRealpathPaths.clear();
  packageContents.clear();
}
var IPathType = /*#__PURE__*/function (IPathType) {
  IPathType[IPathType["FILE"] = 1] = "FILE";
  IPathType[IPathType["DIRECTORY"] = 2] = "DIRECTORY";
  IPathType[IPathType["OTHER"] = 3] = "OTHER";
  return IPathType;
}(IPathType || {});
const checkedPaths = new Map();
function statSyncCached(path) {
  const result = checkedPaths.get(path);
  if (result != null) {
    return result;
  }
  let stat;
  try {
    stat = fs().statSync(path, {
      throwIfNoEntry: false
    });
  } catch (error) {
    if (!(error && (error.code === 'ENOENT' || error.code === 'ENOTDIR'))) {
      throw error;
    }
  }
  if (stat) {
    if (stat.isFile() || stat.isFIFO()) {
      checkedPaths.set(path, IPathType.FILE);
      return IPathType.FILE;
    } else if (stat.isDirectory()) {
      checkedPaths.set(path, IPathType.DIRECTORY);
      return IPathType.DIRECTORY;
    }
  }
  checkedPaths.set(path, IPathType.OTHER);
  return IPathType.OTHER;
}
const checkedRealpathPaths = new Map();
function realpathCached(path) {
  let result = checkedRealpathPaths.get(path);
  if (result != null) {
    return result;
  }
  result = (0, _jestUtil().tryRealpath)(path);
  checkedRealpathPaths.set(path, result);
  if (path !== result) {
    // also cache the result in case it's ever referenced directly - no reason to `realpath` that as well
    checkedRealpathPaths.set(result, result);
  }
  return result;
}
const packageContents = new Map();
function readPackageCached(path) {
  let result = packageContents.get(path);
  if (result != null) {
    return result;
  }
  result = JSON.parse(fs().readFileSync(path, 'utf8'));
  packageContents.set(path, result);
  return result;
}

// adapted from
// https://github.com/lukeed/escalade/blob/2477005062cdbd8407afc90d3f48f4930354252b/src/sync.js
// to use cached `fs` calls
function findClosestPackageJson(start) {
  let dir = (0, _nodePath().resolve)('.', start);
  if (!isDirectory(dir)) {
    dir = (0, _nodePath().dirname)(dir);
  }
  while (true) {
    const pkgJsonFile = (0, _nodePath().resolve)(dir, './package.json');
    const hasPackageJson = isFile(pkgJsonFile);
    if (hasPackageJson) {
      return pkgJsonFile;
    }
    const prevDir = dir;
    dir = (0, _nodePath().dirname)(dir);
    if (prevDir === dir) {
      return undefined;
    }
  }
}

/*
 * helper functions
 */
function isFile(file) {
  return statSyncCached(file) === IPathType.FILE;
}
function isDirectory(dir) {
  return statSyncCached(dir) === IPathType.DIRECTORY;
}
function realpathSync(file) {
  return realpathCached(file);
}

/***/ },

/***/ "./src/index.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
var _exportNames = {};
exports["default"] = void 0;
var _resolver = _interopRequireDefault(__webpack_require__("./src/resolver.ts"));
var _utils = __webpack_require__("./src/utils.ts");
Object.keys(_utils).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
  if (key in exports && exports[key] === _utils[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _utils[key];
    }
  });
});
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
var _default = exports["default"] = _resolver.default;

/***/ },

/***/ "./src/nodeModulesPaths.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.GlobalPaths = void 0;
exports["default"] = nodeModulesPaths;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
 *
 * Adapted from: https://github.com/substack/node-resolve
 */

function nodeModulesPaths(basedir, options) {
  const modules = options && options.moduleDirectory ? [...options.moduleDirectory] : ['node_modules'];

  // ensure that `basedir` is an absolute path at this point,
  // resolving against the process' current working directory
  const basedirAbs = path().resolve(basedir);
  let prefix = '/';
  if (/^([A-Za-z]:)/.test(basedirAbs)) {
    prefix = '';
  } else if (/^\\\\/.test(basedirAbs)) {
    prefix = '\\\\';
  }

  // The node resolution algorithm (as implemented by NodeJS and TypeScript)
  // traverses parents of the physical path, not the symlinked path
  let physicalBasedir;
  try {
    physicalBasedir = (0, _jestUtil().tryRealpath)(basedirAbs);
  } catch {
    // realpath can throw, e.g. on mapped drives
    physicalBasedir = basedirAbs;
  }
  const paths = [physicalBasedir];
  let parsed = path().parse(physicalBasedir);
  while (parsed.dir !== paths.at(-1)) {
    paths.push(parsed.dir);
    parsed = path().parse(parsed.dir);
  }
  const dirs = paths.reduce((dirs, aPath) => {
    for (const moduleDir of modules) {
      if (path().isAbsolute(moduleDir)) {
        if (aPath === basedirAbs && moduleDir) {
          dirs.push(moduleDir);
        }
      } else {
        dirs.push(path().join(prefix, aPath, moduleDir));
      }
    }
    return dirs;
  }, []);
  if (options.paths) {
    dirs.push(...options.paths);
  }
  return dirs;
}
function findGlobalPaths() {
  const {
    root
  } = path().parse(process.cwd());
  const globalPath = path().join(root, 'node_modules');
  const resolvePaths = require.resolve.paths('/');
  if (resolvePaths) {
    // the global paths start one after the root node_modules
    const rootIndex = resolvePaths.indexOf(globalPath);
    return rootIndex === -1 ? [] : resolvePaths.slice(rootIndex + 1);
  }
  return [];
}
const GlobalPaths = exports.GlobalPaths = findGlobalPaths();

/***/ },

/***/ "./src/resolver.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _nodeModule() {
  const data = require("node:module");
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
var _ModuleNotFoundError = _interopRequireDefault(__webpack_require__("./src/ModuleNotFoundError.ts"));
var _defaultResolver = _interopRequireWildcard(__webpack_require__("./src/defaultResolver.ts"));
var _fileWalkers = __webpack_require__("./src/fileWalkers.ts");
var _nodeModulesPaths = _interopRequireWildcard(__webpack_require__("./src/nodeModulesPaths.ts"));
var _shouldLoadAsEsm = _interopRequireWildcard(__webpack_require__("./src/shouldLoadAsEsm.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const NATIVE_PLATFORM = 'native';

// We might be inside a symlink.
const resolvedCwd = (0, _jestUtil().tryRealpath)(process.cwd());
const {
  NODE_PATH
} = process.env;
const nodePaths = NODE_PATH ? NODE_PATH.split(path().delimiter).filter(Boolean)
// The resolver expects absolute paths.
.map(p => path().resolve(resolvedCwd, p)) : undefined;
class Resolver {
  _options;
  _moduleMap;
  _moduleIDCache;
  _moduleNameCache;
  _modulePathCache;
  _supportsNativePlatform;
  _canResolveSync;
  constructor(moduleMap, options) {
    this._options = {
      defaultPlatform: options.defaultPlatform,
      extensions: options.extensions,
      hasCoreModules: options.hasCoreModules === undefined ? true : options.hasCoreModules,
      moduleDirectories: options.moduleDirectories || ['node_modules'],
      moduleNameMapper: options.moduleNameMapper,
      modulePaths: options.modulePaths,
      platforms: options.platforms,
      resolver: options.resolver,
      rootDir: options.rootDir
    };
    this._supportsNativePlatform = options.platforms ? options.platforms.includes(NATIVE_PLATFORM) : false;
    this._moduleMap = moduleMap;
    this._moduleIDCache = new Map();
    this._moduleNameCache = new Map();
    this._modulePathCache = new Map();
  }
  static ModuleNotFoundError = _ModuleNotFoundError.default;
  static tryCastModuleNotFoundError(error) {
    if (error instanceof _ModuleNotFoundError.default) {
      return error;
    }
    const casted = error;
    if (casted.code === 'MODULE_NOT_FOUND') {
      return _ModuleNotFoundError.default.duckType(casted);
    }
    return null;
  }
  static clearDefaultResolverCache() {
    (0, _fileWalkers.clearFsCache)();
    (0, _shouldLoadAsEsm.clearCachedLookups)();
  }
  static findNodeModule(path, options) {
    const resolverModule = loadResolver(options.resolver);
    let resolver = _defaultResolver.default;
    if (typeof resolverModule === 'function') {
      resolver = resolverModule;
    } else if (typeof resolverModule.sync === 'function') {
      resolver = resolverModule.sync;
    }
    const paths = options.paths;
    try {
      return resolver(path, {
        basedir: options.basedir,
        conditions: options.conditions,
        defaultAsyncResolver: _defaultResolver.defaultAsyncResolver,
        defaultResolver: _defaultResolver.default,
        extensions: options.extensions,
        moduleDirectory: options.moduleDirectory,
        paths: paths ? [...(nodePaths || []), ...paths] : nodePaths,
        rootDir: options.rootDir
      });
    } catch (error) {
      // we always wanna throw if it's an internal import
      if (options.throwIfNotFound || path.startsWith('#')) {
        throw error;
      }
    }
    return null;
  }
  static async findNodeModuleAsync(path, options) {
    const resolverModule = loadResolver(options.resolver);
    let resolver = _defaultResolver.defaultAsyncResolver;
    if (typeof resolverModule === 'function') {
      resolver = resolverModule;
    } else if (typeof resolverModule.async === 'function' || typeof resolverModule.sync === 'function') {
      const asyncOrSync = resolverModule.async || resolverModule.sync;
      if (asyncOrSync == null) {
        throw new Error(`Unable to load resolver at ${options.resolver}`);
      }
      resolver = asyncOrSync;
    }
    const paths = options.paths;
    try {
      const result = await resolver(path, {
        basedir: options.basedir,
        conditions: options.conditions,
        defaultAsyncResolver: _defaultResolver.defaultAsyncResolver,
        defaultResolver: _defaultResolver.default,
        extensions: options.extensions,
        moduleDirectory: options.moduleDirectory,
        paths: paths ? [...(nodePaths || []), ...paths] : nodePaths,
        rootDir: options.rootDir
      });
      return result;
    } catch (error) {
      // we always wanna throw if it's an internal import
      if (options.throwIfNotFound || path.startsWith('#')) {
        throw error;
      }
    }
    return null;
  }

  // unstable as it should be replaced by https://github.com/nodejs/modules/issues/393, and we don't want people to use it
  static unstable_shouldLoadAsEsm = _shouldLoadAsEsm.default;
  resolveModuleFromDirIfExists(dirname, moduleName, options) {
    const {
      extensions,
      key,
      moduleDirectory,
      paths,
      skipResolution
    } = this._prepareForResolution(dirname, moduleName, options);
    let module;

    // 1. If we have already resolved this module for this directory name,
    // return a value from the cache.
    const cacheResult = this._moduleNameCache.get(key);
    if (cacheResult) {
      return cacheResult;
    }

    // 2. Check if the module is a haste module.
    module = this.getModule(moduleName);
    if (module) {
      this._moduleNameCache.set(key, module);
      return module;
    }

    // 3. Check if the module is a node module and resolve it based on
    // the node module resolution algorithm. If skipNodeResolution is given we
    // ignore all modules that look like node modules (ie. are not relative
    // requires). This enables us to speed up resolution when we build a
    // dependency graph because we don't have to look at modules that may not
    // exist and aren't mocked.
    const resolveNodeModule = (name, throwIfNotFound = false) => {
      // Only skip default resolver
      if (this.isCoreModule(name) && !this._options.resolver) {
        return name;
      }
      return Resolver.findNodeModule(name, {
        basedir: dirname,
        conditions: options?.conditions,
        extensions,
        moduleDirectory,
        paths,
        resolver: this._options.resolver,
        rootDir: this._options.rootDir,
        throwIfNotFound
      });
    };
    if (!skipResolution) {
      module = resolveNodeModule(moduleName, Boolean(process.versions.pnp));
      if (module) {
        this._moduleNameCache.set(key, module);
        return module;
      }
    }

    // 4. Resolve "haste packages" which are `package.json` files outside of
    // `node_modules` folders anywhere in the file system.
    try {
      const hasteModulePath = this._getHasteModulePath(moduleName);
      if (hasteModulePath) {
        // try resolving with custom resolver first to support extensions,
        // then fallback to require.resolve
        const resolvedModule = resolveNodeModule(hasteModulePath) || require.resolve(hasteModulePath);
        this._moduleNameCache.set(key, resolvedModule);
        return resolvedModule;
      }
    } catch {}
    return null;
  }
  async resolveModuleFromDirIfExistsAsync(dirname, moduleName, options) {
    const {
      extensions,
      key,
      moduleDirectory,
      paths,
      skipResolution
    } = this._prepareForResolution(dirname, moduleName, options);
    let module;

    // 1. If we have already resolved this module for this directory name,
    // return a value from the cache.
    const cacheResult = this._moduleNameCache.get(key);
    if (cacheResult) {
      return cacheResult;
    }

    // 2. Check if the module is a haste module.
    module = this.getModule(moduleName);
    if (module) {
      this._moduleNameCache.set(key, module);
      return module;
    }

    // 3. Check if the module is a node module and resolve it based on
    // the node module resolution algorithm. If skipNodeResolution is given we
    // ignore all modules that look like node modules (ie. are not relative
    // requires). This enables us to speed up resolution when we build a
    // dependency graph because we don't have to look at modules that may not
    // exist and aren't mocked.
    const resolveNodeModule = async (name, throwIfNotFound = false) => {
      // Only skip default resolver
      if (this.isCoreModule(name) && !this._options.resolver) {
        return name;
      }
      return Resolver.findNodeModuleAsync(name, {
        basedir: dirname,
        conditions: options?.conditions,
        extensions,
        moduleDirectory,
        paths,
        resolver: this._options.resolver,
        rootDir: this._options.rootDir,
        throwIfNotFound
      });
    };
    if (!skipResolution) {
      module = await resolveNodeModule(moduleName, Boolean(process.versions.pnp));
      if (module) {
        this._moduleNameCache.set(key, module);
        return module;
      }
    }

    // 4. Resolve "haste packages" which are `package.json` files outside of
    // `node_modules` folders anywhere in the file system.
    try {
      const hasteModulePath = this._getHasteModulePath(moduleName);
      if (hasteModulePath) {
        // try resolving with custom resolver first to support extensions,
        // then fallback to require.resolve
        const resolvedModule = (await resolveNodeModule(hasteModulePath)) ||
        // QUESTION: should this be async?
        require.resolve(hasteModulePath);
        this._moduleNameCache.set(key, resolvedModule);
        return resolvedModule;
      }
    } catch {}
    return null;
  }

  // True when synchronous module resolution is available with the configured
  // resolver. False when a user has configured a resolver that only exports
  // an `async` hook - in that case `findNodeModule` silently falls back to
  // the default resolver, which will not honor the user's mappings, so
  // callers that need correctness should defer to the async API.
  //
  // The contract is "plain function or `.sync` hook = sync"; an `async
  // function` in either slot is a user-side contract violation we don't
  // defend against here.
  //
  // Memoized: the resolver config is immutable per-`Resolver`. A throwing
  // `loadResolver` (malformed user resolver) is treated as `false` so
  // callers route to the async path, which will surface the actual error.
  canResolveSync() {
    if (this._canResolveSync != null) return this._canResolveSync;
    let result;
    try {
      const resolverModule = loadResolver(this._options.resolver);
      result = typeof resolverModule === 'function' || typeof resolverModule.sync === 'function';
    } catch {
      result = false;
    }
    this._canResolveSync = result;
    return result;
  }
  resolveModule(from, moduleName, options) {
    const dirname = path().dirname(from);
    const module = this.resolveStubModuleName(from, moduleName, options) || this.resolveModuleFromDirIfExists(dirname, moduleName, options);
    if (module) return module;

    // 5. Throw an error if the module could not be found. `resolve.sync` only
    // produces an error based on the dirname but we have the actual current
    // module name available.
    this._throwModNotFoundError(from, moduleName);
  }
  async resolveModuleAsync(from, moduleName, options) {
    const dirname = path().dirname(from);
    const module = (await this.resolveStubModuleNameAsync(from, moduleName, options)) || (await this.resolveModuleFromDirIfExistsAsync(dirname, moduleName, options));
    if (module) return module;

    // 5. Throw an error if the module could not be found. `resolve` only
    // produces an error based on the dirname but we have the actual current
    // module name available.
    this._throwModNotFoundError(from, moduleName);
  }

  /**
   * _prepareForResolution is shared between the sync and async module resolution
   * methods, to try to keep them as DRY as possible.
   */
  _prepareForResolution(dirname, moduleName, options) {
    const paths = options?.paths || this._options.modulePaths;
    const moduleDirectory = this._options.moduleDirectories;
    const stringifiedOptions = options ? JSON.stringify(options) : '';
    const key = dirname + path().delimiter + moduleName + stringifiedOptions;
    const defaultPlatform = this._options.defaultPlatform;
    const extensions = [...this._options.extensions];
    if (this._supportsNativePlatform) {
      extensions.unshift(...this._options.extensions.map(ext => `.${NATIVE_PLATFORM}${ext}`));
    }
    if (defaultPlatform) {
      extensions.unshift(...this._options.extensions.map(ext => `.${defaultPlatform}${ext}`));
    }
    const skipResolution = options && options.skipNodeResolution && !moduleName.includes(path().sep);
    return {
      extensions,
      key,
      moduleDirectory,
      paths,
      skipResolution
    };
  }

  /**
   * _getHasteModulePath attempts to return the path to a haste module.
   */
  _getHasteModulePath(moduleName) {
    const parts = moduleName.split('/');
    const hastePackage = this.getPackage(parts.shift());
    if (hastePackage) {
      return path().join(path().dirname(hastePackage), ...parts);
    }
    return null;
  }
  _throwModNotFoundError(from, moduleName) {
    const relativePath = (0, _slash().default)(path().relative(this._options.rootDir, from)) || '.';
    throw new _ModuleNotFoundError.default(`Cannot find module '${moduleName}' from '${relativePath}'`, moduleName);
  }
  _getMapModuleName(matches) {
    return matches ? moduleName => moduleName.replaceAll(/\$(\d+)/g, (_, index) => matches[Number.parseInt(index, 10)] || '') : moduleName => moduleName;
  }
  _isAliasModule(moduleName) {
    const moduleNameMapper = this._options.moduleNameMapper;
    if (!moduleNameMapper) {
      return false;
    }
    return moduleNameMapper.some(({
      regex
    }) => regex.test(moduleName));
  }
  isCoreModule(moduleName) {
    return this._options.hasCoreModules && (0, _nodeModule().isBuiltin)(moduleName) && !this._isAliasModule(moduleName);
  }
  normalizeCoreModuleSpecifier(specifier) {
    return specifier.startsWith('node:') ? specifier.slice(5) : specifier;
  }
  getModule(name) {
    return this._moduleMap.getModule(name, this._options.defaultPlatform, this._supportsNativePlatform);
  }
  getModulePath(from, moduleName) {
    if (moduleName[0] !== '.' || path().isAbsolute(moduleName)) {
      return moduleName;
    }
    return path().normalize(`${path().dirname(from)}/${moduleName}`);
  }
  getPackage(name) {
    return this._moduleMap.getPackage(name, this._options.defaultPlatform, this._supportsNativePlatform);
  }
  getMockModule(from, name, options) {
    const mock = this._moduleMap.getMockModule(name);
    if (mock) {
      return mock;
    } else {
      const resolvedName = this.resolveStubModuleName(from, name, options);
      if (resolvedName) {
        return this._moduleMap.getMockModule(resolvedName) ?? null;
      }
    }
    return null;
  }
  async getMockModuleAsync(from, name, options) {
    const mock = this._moduleMap.getMockModule(name);
    if (mock) {
      return mock;
    } else {
      const resolvedName = await this.resolveStubModuleNameAsync(from, name, options);
      if (resolvedName) {
        return this._moduleMap.getMockModule(resolvedName) ?? null;
      }
    }
    return null;
  }
  getModulePaths(from) {
    const cachedModule = this._modulePathCache.get(from);
    if (cachedModule) {
      return cachedModule;
    }
    const moduleDirectory = this._options.moduleDirectories;
    const paths = (0, _nodeModulesPaths.default)(from, {
      moduleDirectory
    });
    if (paths.at(-1) === undefined) {
      // circumvent node-resolve bug that adds `undefined` as last item.
      paths.pop();
    }
    this._modulePathCache.set(from, paths);
    return paths;
  }
  getGlobalPaths(moduleName) {
    if (!moduleName || moduleName[0] === '.' || this.isCoreModule(moduleName)) {
      return [];
    }
    return _nodeModulesPaths.GlobalPaths;
  }
  getModuleID(virtualMocks, from, moduleName = '', options) {
    const stringifiedOptions = options ? JSON.stringify(options) : '';
    const key = from + path().delimiter + moduleName + stringifiedOptions;
    const cachedModuleID = this._moduleIDCache.get(key);
    if (cachedModuleID) {
      return cachedModuleID;
    }
    const moduleType = this._getModuleType(moduleName);
    const absolutePath = this._getAbsolutePath(virtualMocks, from, moduleName, options);
    const mockPath = this._getMockPath(from, moduleName, options);
    const sep = path().delimiter;
    const id = moduleType + sep + (absolutePath ? absolutePath + sep : '') + (mockPath ? mockPath + sep : '') + (stringifiedOptions ? stringifiedOptions + sep : '');
    this._moduleIDCache.set(key, id);
    return id;
  }
  async getModuleIDAsync(virtualMocks, from, moduleName = '', options) {
    const stringifiedOptions = options ? JSON.stringify(options) : '';
    const key = from + path().delimiter + moduleName + stringifiedOptions;
    const cachedModuleID = this._moduleIDCache.get(key);
    if (cachedModuleID) {
      return cachedModuleID;
    }
    if (moduleName.startsWith('data:')) {
      return moduleName;
    }
    const moduleType = this._getModuleType(moduleName);
    const absolutePath = await this._getAbsolutePathAsync(virtualMocks, from, moduleName, options);
    const mockPath = await this._getMockPathAsync(from, moduleName, options);
    const sep = path().delimiter;
    const id = moduleType + sep + (absolutePath ? absolutePath + sep : '') + (mockPath ? mockPath + sep : '') + (stringifiedOptions ? stringifiedOptions + sep : '');
    this._moduleIDCache.set(key, id);
    return id;
  }
  _getModuleType(moduleName) {
    return this.isCoreModule(moduleName) ? 'node' : 'user';
  }
  _getAbsolutePath(virtualMocks, from, moduleName, options) {
    if (this.isCoreModule(moduleName)) {
      return this.normalizeCoreModuleSpecifier(moduleName);
    }
    if (moduleName.startsWith('data:')) {
      return moduleName;
    }
    return this._isModuleResolved(from, moduleName, options) ? this.getModule(moduleName) : this._getVirtualMockPath(virtualMocks, from, moduleName, options);
  }
  async _getAbsolutePathAsync(virtualMocks, from, moduleName, options) {
    if (this.isCoreModule(moduleName)) {
      return this.normalizeCoreModuleSpecifier(moduleName);
    }
    if (moduleName.startsWith('data:')) {
      return moduleName;
    }
    const isModuleResolved = await this._isModuleResolvedAsync(from, moduleName, options);
    return isModuleResolved ? this.getModule(moduleName) : this._getVirtualMockPathAsync(virtualMocks, from, moduleName, options);
  }
  _getMockPath(from, moduleName, options) {
    return this.isCoreModule(moduleName) ? null : this.getMockModule(from, moduleName, options);
  }
  async _getMockPathAsync(from, moduleName, options) {
    return this.isCoreModule(moduleName) ? null : this.getMockModuleAsync(from, moduleName, options);
  }
  _getVirtualMockPath(virtualMocks, from, moduleName, options) {
    const virtualMockPath = this.getModulePath(from, moduleName);
    return virtualMocks.get(virtualMockPath) ? virtualMockPath : moduleName ? this.resolveModule(from, moduleName, options) : from;
  }
  async _getVirtualMockPathAsync(virtualMocks, from, moduleName, options) {
    const virtualMockPath = this.getModulePath(from, moduleName);
    return virtualMocks.get(virtualMockPath) ? virtualMockPath : moduleName ? this.resolveModuleAsync(from, moduleName, options) : from;
  }
  _isModuleResolved(from, moduleName, options) {
    return !!(this.getModule(moduleName) || this.getMockModule(from, moduleName, options));
  }
  async _isModuleResolvedAsync(from, moduleName, options) {
    return !!(this.getModule(moduleName) || (await this.getMockModuleAsync(from, moduleName, options)));
  }
  resolveStubModuleName(from, moduleName, options) {
    const dirname = path().dirname(from);
    const {
      extensions,
      moduleDirectory,
      paths
    } = this._prepareForResolution(dirname, moduleName);
    const moduleNameMapper = this._options.moduleNameMapper;
    const resolver = this._options.resolver;
    if (moduleNameMapper) {
      for (const {
        moduleName: mappedModuleName,
        regex
      } of moduleNameMapper) {
        if (regex.test(moduleName)) {
          // Note: once a moduleNameMapper matches the name, it must result
          // in a module, or else an error is thrown.
          const matches = moduleName.match(regex);
          const mapModuleName = this._getMapModuleName(matches);
          const possibleModuleNames = Array.isArray(mappedModuleName) ? mappedModuleName : [mappedModuleName];
          let module = null;
          for (const possibleModuleName of possibleModuleNames) {
            const updatedName = mapModuleName(possibleModuleName);
            module = this.getModule(updatedName) || Resolver.findNodeModule(updatedName, {
              basedir: dirname,
              conditions: options?.conditions,
              extensions,
              moduleDirectory,
              paths,
              resolver,
              rootDir: this._options.rootDir
            });
            if (module) {
              break;
            }
          }
          if (!module) {
            throw createNoMappedModuleFoundError(moduleName, mapModuleName, mappedModuleName, regex, resolver);
          }
          return module;
        }
      }
    }
    return null;
  }
  async resolveStubModuleNameAsync(from, moduleName, options) {
    // Strip node URL scheme from core modules imported using it
    if (this.isCoreModule(moduleName)) {
      return this.normalizeCoreModuleSpecifier(moduleName);
    }
    const dirname = path().dirname(from);
    const {
      extensions,
      moduleDirectory,
      paths
    } = this._prepareForResolution(dirname, moduleName);
    const moduleNameMapper = this._options.moduleNameMapper;
    const resolver = this._options.resolver;
    if (moduleNameMapper) {
      for (const {
        moduleName: mappedModuleName,
        regex
      } of moduleNameMapper) {
        if (regex.test(moduleName)) {
          // Note: once a moduleNameMapper matches the name, it must result
          // in a module, or else an error is thrown.
          const matches = moduleName.match(regex);
          const mapModuleName = this._getMapModuleName(matches);
          const possibleModuleNames = Array.isArray(mappedModuleName) ? mappedModuleName : [mappedModuleName];
          let module = null;
          for (const possibleModuleName of possibleModuleNames) {
            const updatedName = mapModuleName(possibleModuleName);
            module = this.getModule(updatedName) || (await Resolver.findNodeModuleAsync(updatedName, {
              basedir: dirname,
              conditions: options?.conditions,
              extensions,
              moduleDirectory,
              paths,
              resolver,
              rootDir: this._options.rootDir
            }));
            if (module) {
              break;
            }
          }
          if (!module) {
            throw createNoMappedModuleFoundError(moduleName, mapModuleName, mappedModuleName, regex, resolver);
          }
          return module;
        }
      }
    }
    return null;
  }
}
exports["default"] = Resolver;
const createNoMappedModuleFoundError = (moduleName, mapModuleName, mappedModuleName, regex, resolver) => {
  const mappedAs = Array.isArray(mappedModuleName) ? JSON.stringify(mappedModuleName.map(mapModuleName), null, 2) : mappedModuleName;
  const original = Array.isArray(mappedModuleName) ? `${JSON.stringify(mappedModuleName, null, 6) // using 6 because of misalignment when nested below
  .slice(0, -1) + ' '.repeat(4)}]` /// align last bracket correctly as well
  : mappedModuleName;
  const error = new Error(_chalk().default.red(`${_chalk().default.bold('Configuration error')}:

Could not locate module ${_chalk().default.bold(moduleName)} mapped as:
${_chalk().default.bold(mappedAs)}.

Please check your configuration for these entries:
{
  "moduleNameMapper": {
    "${regex.toString()}": "${_chalk().default.bold(original)}"
  },
  "resolver": ${_chalk().default.bold(String(resolver))}
}`));
  error.name = '';
  return error;
};
function loadResolver(resolver) {
  if (resolver == null) {
    return _defaultResolver.default;
  }
  const loadedResolver = require(resolver);
  if (loadedResolver == null) {
    throw new Error(`Resolver located at ${resolver} does not export anything`);
  }
  if (typeof loadedResolver === 'function') {
    return loadedResolver;
  }
  if (typeof loadedResolver === 'object' && (loadedResolver.sync != null || loadedResolver.async != null)) {
    return loadedResolver;
  }
  throw new Error(`Resolver located at ${resolver} does not export a function or an object with "sync" and "async" props`);
}

/***/ },

/***/ "./src/shouldLoadAsEsm.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.clearCachedLookups = clearCachedLookups;
exports["default"] = cachedShouldLoadAsEsm;
function _nodePath() {
  const data = require("node:path");
  _nodePath = function () {
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
var _fileWalkers = __webpack_require__("./src/fileWalkers.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const runtimeSupportsVmModules = typeof _nodeVm().SyntheticModule === 'function';
const cachedFileLookups = new Map();
const cachedDirLookups = new Map();
const cachedChecks = new Map();
function clearCachedLookups() {
  cachedFileLookups.clear();
  cachedDirLookups.clear();
  cachedChecks.clear();
}
function cachedShouldLoadAsEsm(path, extensionsToTreatAsEsm) {
  if (!runtimeSupportsVmModules) {
    return false;
  }
  let cachedLookup = cachedFileLookups.get(path);
  if (cachedLookup === undefined) {
    cachedLookup = shouldLoadAsEsm(path, extensionsToTreatAsEsm);
    cachedFileLookups.set(path, cachedLookup);
  }
  return cachedLookup;
}

// this is a bad version of what https://github.com/nodejs/modules/issues/393 would provide
function shouldLoadAsEsm(path, extensionsToTreatAsEsm) {
  const extension = (0, _nodePath().extname)(path);
  if (extension === '.mjs') {
    return true;
  }
  if (extension === '.cjs') {
    return false;
  }
  if (extension !== '.js') {
    return extensionsToTreatAsEsm.includes(extension);
  }
  const cwd = (0, _nodePath().dirname)(path);
  let cachedLookup = cachedDirLookups.get(cwd);
  if (cachedLookup === undefined) {
    cachedLookup = cachedPkgCheck(cwd);
    cachedFileLookups.set(cwd, cachedLookup);
  }
  return cachedLookup;
}
function cachedPkgCheck(cwd) {
  const pkgPath = (0, _fileWalkers.findClosestPackageJson)(cwd);
  if (!pkgPath) {
    return false;
  }
  let hasModuleField = cachedChecks.get(pkgPath);
  if (hasModuleField != null) {
    return hasModuleField;
  }
  try {
    const pkg = (0, _fileWalkers.readPackageCached)(pkgPath);
    hasModuleField = pkg.type === 'module';
  } catch {
    hasModuleField = false;
  }
  cachedChecks.set(pkgPath, hasModuleField);
  return hasModuleField;
}

/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.resolveWatchPlugin = exports.resolveTestEnvironment = exports.resolveSequencer = exports.resolveRunner = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _jestValidate() {
  const data = require("jest-validate");
  _jestValidate = function () {
    return data;
  };
  return data;
}
var _resolver = _interopRequireDefault(__webpack_require__("./src/resolver.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const BULLET = _chalk().default.bold('\u25CF ');
const DOCUMENTATION_NOTE = `  ${_chalk().default.bold('Configuration Documentation:')}
  https://jestjs.io/docs/configuration
`;
const createValidationError = message => new (_jestValidate().ValidationError)(`${BULLET}Validation Error`, message, DOCUMENTATION_NOTE);
const replaceRootDirInPath = (rootDir, filePath) => {
  if (!filePath.startsWith('<rootDir>')) {
    return filePath;
  }
  return path().resolve(rootDir, path().normalize(`./${filePath.slice('<rootDir>'.length)}`));
};
const resolveWithPrefix = (resolver, {
  filePath,
  humanOptionName,
  optionName,
  prefix,
  requireResolveFunction,
  rootDir
}) => {
  const fileName = replaceRootDirInPath(rootDir, filePath);
  let module = _resolver.default.findNodeModule(`${prefix}${fileName}`, {
    basedir: rootDir,
    resolver: resolver || undefined
  });
  if (module) {
    return module;
  }
  try {
    return requireResolveFunction(`${prefix}${fileName}`);
  } catch {}
  module = _resolver.default.findNodeModule(fileName, {
    basedir: rootDir,
    resolver: resolver || undefined
  });
  if (module) {
    return module;
  }
  try {
    return requireResolveFunction(fileName);
  } catch {}
  throw createValidationError(`  ${humanOptionName} ${_chalk().default.bold(fileName)} cannot be found. Make sure the ${_chalk().default.bold(optionName)} configuration option points to an existing node module.`);
};

/**
 * Finds the test environment to use:
 *
 * 1. looks for jest-environment-<name> relative to project.
 * 1. looks for jest-environment-<name> relative to Jest.
 * 1. looks for <name> relative to project.
 * 1. looks for <name> relative to Jest.
 */
const resolveTestEnvironment = ({
  rootDir,
  testEnvironment: filePath,
  requireResolveFunction
}) => {
  // we don't want to resolve the actual `jsdom` module if `jest-environment-jsdom` is not installed, but `jsdom` package is
  if (filePath === 'jsdom') {
    filePath = 'jest-environment-jsdom';
  }
  try {
    return resolveWithPrefix(undefined, {
      filePath,
      humanOptionName: 'Test environment',
      optionName: 'testEnvironment',
      prefix: 'jest-environment-',
      requireResolveFunction,
      rootDir
    });
  } catch (error) {
    if (filePath === 'jest-environment-jsdom') {
      error.message += '\n\nAs of Jest 28 "jest-environment-jsdom" is no longer shipped by default, make sure to install it separately.';
    }
    throw error;
  }
};

/**
 * Finds the watch plugins to use:
 *
 * 1. looks for jest-watch-<name> relative to project.
 * 1. looks for jest-watch-<name> relative to Jest.
 * 1. looks for <name> relative to project.
 * 1. looks for <name> relative to Jest.
 */
exports.resolveTestEnvironment = resolveTestEnvironment;
const resolveWatchPlugin = (resolver, {
  filePath,
  rootDir,
  requireResolveFunction
}) => resolveWithPrefix(resolver, {
  filePath,
  humanOptionName: 'Watch plugin',
  optionName: 'watchPlugins',
  prefix: 'jest-watch-',
  requireResolveFunction,
  rootDir
});

/**
 * Finds the runner to use:
 *
 * 1. looks for jest-runner-<name> relative to project.
 * 1. looks for jest-runner-<name> relative to Jest.
 * 1. looks for <name> relative to project.
 * 1. looks for <name> relative to Jest.
 */
exports.resolveWatchPlugin = resolveWatchPlugin;
const resolveRunner = (resolver, {
  filePath,
  rootDir,
  requireResolveFunction
}) => resolveWithPrefix(resolver, {
  filePath,
  humanOptionName: 'Jest Runner',
  optionName: 'runner',
  prefix: 'jest-runner-',
  requireResolveFunction,
  rootDir
});
exports.resolveRunner = resolveRunner;
const resolveSequencer = (resolver, {
  filePath,
  rootDir,
  requireResolveFunction
}) => resolveWithPrefix(resolver, {
  filePath,
  humanOptionName: 'Jest Sequencer',
  optionName: 'testSequencer',
  prefix: 'jest-sequencer-',
  requireResolveFunction,
  rootDir
});
exports.resolveSequencer = resolveSequencer;

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
/******/ 	
/******/ 	// startup
/******/ 	// Load entry module and return exports
/******/ 	// This entry module is referenced by other modules so it can't be inlined
/******/ 	var __webpack_exports__ = __webpack_require__("./src/index.ts");
/******/ 	module.exports = __webpack_exports__;
/******/ 	
/******/ })()
;