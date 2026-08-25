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

/***/ "./src/babel.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
Object.defineProperty(exports, "loadPartialConfigAsync", ({
  enumerable: true,
  get: function () {
    return _core().loadPartialConfigAsync;
  }
}));
exports.loadPartialConfigSync = void 0;
Object.defineProperty(exports, "transformAsync", ({
  enumerable: true,
  get: function () {
    return _core().transformAsync;
  }
}));
Object.defineProperty(exports, "transformSync", ({
  enumerable: true,
  get: function () {
    return _core().transformSync;
  }
}));
function _core() {
  const data = require("@babel/core");
  _core = function () {
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

// this is a separate file so it can be mocked in tests

// Old babel 7 versions didn't have loadPartialConfigSync
const _loadPartialConfigSync = exports.loadPartialConfigSync = _core().loadPartialConfigSync ?? _core().loadPartialConfig;

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
exports["default"] = exports.createTransformer = void 0;
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
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
function _slash() {
  const data = _interopRequireDefault(require("slash"));
  _slash = function () {
    return data;
  };
  return data;
}
var _babel = __webpack_require__("./src/babel.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const THIS_FILE = fs().readFileSync(__filename);
const jestPresetPath = require.resolve('babel-preset-jest');
const babelIstanbulPlugin = require.resolve('babel-plugin-istanbul');
function assertLoadedBabelConfig(babelConfig, cwd, filename) {
  if (!babelConfig) {
    throw new Error(`babel-jest: Babel ignores ${_chalk().default.bold((0, _slash().default)(path().relative(cwd, filename)))} - make sure to include the file in Jest's ${_chalk().default.bold('transformIgnorePatterns')} as well.`);
  }
}
function addIstanbulInstrumentation(filename, babelOptions, transformOptions) {
  if (transformOptions.instrument) {
    const copiedBabelOptions = {
      ...babelOptions,
      auxiliaryCommentBefore: ' istanbul ignore next '
    };
    // Copied from jest-runtime transform.js
    copiedBabelOptions.plugins = [...(copiedBabelOptions.plugins ?? []), [babelIstanbulPlugin, {
      // files outside `cwd` will not be instrumented
      cwd: transformOptions.config.cwd,
      exclude: [],
      extension: [path().extname(filename)]
    }]];
    return copiedBabelOptions;
  }
  return babelOptions;
}
function getCacheKeyFromConfig(sourceText, sourcePath, babelOptions, transformOptions) {
  const {
    config,
    configString,
    instrument
  } = transformOptions;
  const configPath = [babelOptions.config ?? '', babelOptions.babelrc ?? ''];
  return (0, _nodeCrypto().createHash)('sha1').update(THIS_FILE).update('\0', 'utf8').update(JSON.stringify(babelOptions.options)).update('\0', 'utf8').update(sourceText).update('\0', 'utf8').update(path().relative(config.rootDir, sourcePath)).update('\0', 'utf8').update(configString).update('\0', 'utf8').update(configPath.join('')).update('\0', 'utf8').update(instrument ? 'instrument' : '').update('\0', 'utf8').update("production" ?? 0).update('\0', 'utf8').update(process.env.BABEL_ENV ?? '').update('\0', 'utf8').update(process.version).digest('hex').slice(0, 32);
}
function loadBabelConfig(cwd, filename, transformOptions) {
  const babelConfig = (0, _babel.loadPartialConfigSync)(transformOptions);
  assertLoadedBabelConfig(babelConfig, cwd, filename);
  return babelConfig;
}
async function loadBabelConfigAsync(cwd, filename, transformOptions) {
  const babelConfig = await (0, _babel.loadPartialConfigAsync)(transformOptions);
  assertLoadedBabelConfig(babelConfig, cwd, filename);
  return babelConfig;
}
function loadBabelOptions(cwd, filename, transformOptions, jestTransformOptions) {
  const {
    options
  } = loadBabelConfig(cwd, filename, transformOptions);
  return addIstanbulInstrumentation(filename, options, jestTransformOptions);
}
async function loadBabelOptionsAsync(cwd, filename, transformOptions, jestTransformOptions) {
  const {
    options
  } = await loadBabelConfigAsync(cwd, filename, transformOptions);
  return addIstanbulInstrumentation(filename, options, jestTransformOptions);
}
const createTransformer = transformerConfig => {
  const {
    excludeJestPreset,
    ...inputOptions
  } = transformerConfig ?? {};
  const options = {
    ...inputOptions,
    caller: {
      name: 'babel-jest',
      supportsDynamicImport: false,
      supportsExportNamespaceFrom: false,
      supportsStaticESM: false,
      supportsTopLevelAwait: false,
      ...inputOptions.caller
    },
    compact: false,
    plugins: inputOptions.plugins ?? [],
    presets: [...(inputOptions.presets ?? []), ...(excludeJestPreset === true ? [] : [jestPresetPath])],
    sourceMaps: 'both'
  };
  function mergeBabelTransformOptions(filename, transformOptions) {
    const {
      cwd,
      rootDir
    } = transformOptions.config;
    // `cwd` and `root` first to allow incoming options to override it
    return {
      cwd,
      root: rootDir,
      ...options,
      caller: {
        ...options.caller,
        supportsDynamicImport: transformOptions.supportsDynamicImport ?? options.caller.supportsDynamicImport,
        supportsExportNamespaceFrom: transformOptions.supportsExportNamespaceFrom ?? options.caller.supportsExportNamespaceFrom,
        supportsStaticESM: transformOptions.supportsStaticESM ?? options.caller.supportsStaticESM,
        supportsTopLevelAwait: transformOptions.supportsTopLevelAwait ?? options.caller.supportsTopLevelAwait
      },
      filename
    };
  }
  return {
    canInstrument: true,
    getCacheKey(sourceText, sourcePath, transformOptions) {
      const babelOptions = loadBabelConfig(transformOptions.config.cwd, sourcePath, mergeBabelTransformOptions(sourcePath, transformOptions));
      return getCacheKeyFromConfig(sourceText, sourcePath, babelOptions, transformOptions);
    },
    async getCacheKeyAsync(sourceText, sourcePath, transformOptions) {
      const babelOptions = await loadBabelConfigAsync(transformOptions.config.cwd, sourcePath, mergeBabelTransformOptions(sourcePath, transformOptions));
      return getCacheKeyFromConfig(sourceText, sourcePath, babelOptions, transformOptions);
    },
    process(sourceText, sourcePath, transformOptions) {
      const babelOptions = loadBabelOptions(transformOptions.config.cwd, sourcePath, mergeBabelTransformOptions(sourcePath, transformOptions), transformOptions);
      const transformResult = (0, _babel.transformSync)(sourceText, babelOptions);
      if (transformResult) {
        const {
          code,
          map
        } = transformResult;
        if (typeof code === 'string') {
          return {
            code,
            map
          };
        }
      }
      return {
        code: sourceText
      };
    },
    async processAsync(sourceText, sourcePath, transformOptions) {
      const babelOptions = await loadBabelOptionsAsync(transformOptions.config.cwd, sourcePath, mergeBabelTransformOptions(sourcePath, transformOptions), transformOptions);
      const transformResult = await (0, _babel.transformAsync)(sourceText, babelOptions);
      if (transformResult) {
        const {
          code,
          map
        } = transformResult;
        if (typeof code === 'string') {
          return {
            code,
            map
          };
        }
      }
      return {
        code: sourceText
      };
    }
  };
};
exports.createTransformer = createTransformer;
const transformerFactory = {
  // Assigned here, instead of as a separate export, due to limitations in Jest's
  // requireOrImportModule, requiring all exports to be on the `default` export
  createTransformer
};
var _default = exports["default"] = transformerFactory;
})();

module.exports = __webpack_exports__;
/******/ })()
;