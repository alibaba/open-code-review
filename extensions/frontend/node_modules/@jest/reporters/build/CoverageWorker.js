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

/***/ "./src/generateEmptyCoverage.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = generateEmptyCoverage;
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
    return data;
  };
  return data;
}
function _istanbulLibCoverage() {
  const data = require("istanbul-lib-coverage");
  _istanbulLibCoverage = function () {
    return data;
  };
  return data;
}
function _istanbulLibInstrument() {
  const data = require("istanbul-lib-instrument");
  _istanbulLibInstrument = function () {
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
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

async function generateEmptyCoverage(source, filename, globalConfig, config, changedFiles, sourcesRelatedToTestsInChangedFiles) {
  const coverageOptions = {
    changedFiles,
    collectCoverage: globalConfig.collectCoverage,
    collectCoverageFrom: globalConfig.collectCoverageFrom,
    coverageProvider: globalConfig.coverageProvider,
    globalRootDir: globalConfig.rootDir,
    sourcesRelatedToTestsInChangedFiles
  };
  let coverageWorkerResult = null;
  if ((0, _transform().shouldInstrument)(filename, coverageOptions, config)) {
    if (coverageOptions.coverageProvider === 'v8') {
      const stat = fs().statSync(filename);
      return {
        kind: 'V8Coverage',
        result: {
          functions: [{
            functionName: '(empty-report)',
            isBlockCoverage: true,
            ranges: [{
              count: 0,
              endOffset: stat.size,
              startOffset: 0
            }]
          }],
          scriptId: '0',
          url: filename
        }
      };
    }
    const scriptTransformer = await (0, _transform().createScriptTransformer)(config);

    // Transform file with instrumentation to make sure initial coverage data is well mapped to original code.
    const {
      code
    } = await scriptTransformer.transformSourceAsync(filename, source, {
      instrument: true,
      supportsDynamicImport: true,
      supportsExportNamespaceFrom: true,
      supportsStaticESM: true,
      supportsTopLevelAwait: true
    });
    // TODO: consider passing AST
    const extracted = (0, _istanbulLibInstrument().readInitialCoverage)(code);
    // Check extracted initial coverage is not null, this can happen when using /* istanbul ignore file */
    if (extracted) {
      coverageWorkerResult = {
        coverage: (0, _istanbulLibCoverage().createFileCoverage)(extracted.coverageData),
        kind: 'BabelCoverage'
      };
    }
  }
  return coverageWorkerResult;
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
exports.worker = worker;
function _exitX() {
  const data = _interopRequireDefault(require("exit-x"));
  _exitX = function () {
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
var _generateEmptyCoverage = _interopRequireDefault(__webpack_require__("./src/generateEmptyCoverage.ts"));
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Make sure uncaught errors are logged before we exit.
process.on('uncaughtException', err => {
  if (err.stack) {
    console.error(err.stack);
  } else {
    console.error(err);
  }
  (0, _exitX().default)(1);
});
function worker({
  config,
  globalConfig,
  path,
  context
}) {
  return (0, _generateEmptyCoverage.default)(fs().readFileSync(path, 'utf8'), path, globalConfig, config, context.changedFiles && new Set(context.changedFiles), context.sourcesRelatedToTestsInChangedFiles && new Set(context.sourcesRelatedToTestsInChangedFiles));
}
})();

module.exports = __webpack_exports__;
/******/ })()
;