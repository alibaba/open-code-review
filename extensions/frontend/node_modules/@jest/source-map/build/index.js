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

/***/ "./src/getCallsite.ts":
/***/ ((__unused_webpack_module, exports) => {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getCallsite;
function _traceMapping() {
  const data = require("@jridgewell/trace-mapping");
  _traceMapping = function () {
    return data;
  };
  return data;
}
function _callsites() {
  const data = _interopRequireDefault(require("callsites"));
  _callsites = function () {
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
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Copied from https://github.com/rexxars/sourcemap-decorate-callsites/blob/5b9735a156964973a75dc62fd2c7f0c1975458e8/lib/index.js#L113-L158
const addSourceMapConsumer = (callsite, tracer) => {
  const getLineNumber = callsite.getLineNumber.bind(callsite);
  const getColumnNumber = callsite.getColumnNumber.bind(callsite);
  let position = null;
  function getPosition() {
    position ??= (0, _traceMapping().originalPositionFor)(tracer, {
      column: getColumnNumber() ?? -1,
      line: getLineNumber() ?? -1
    });
    return position;
  }
  Object.defineProperties(callsite, {
    getColumnNumber: {
      value() {
        const value = getPosition().column;
        return value == null || value === 0 ? getColumnNumber() : value;
      },
      writable: false
    },
    getLineNumber: {
      value() {
        const value = getPosition().line;
        return value == null || value === 0 ? getLineNumber() : value;
      },
      writable: false
    }
  });
};
function getCallsite(level, sourceMaps) {
  const levelAfterThisCall = level + 1;
  const stack = (0, _callsites().default)()[levelAfterThisCall];
  const sourceMapFileName = sourceMaps?.get(stack.getFileName() ?? '');
  if (sourceMapFileName != null && sourceMapFileName !== '') {
    try {
      const sourceMap = (0, _gracefulFs().readFileSync)(sourceMapFileName, 'utf8');
      addSourceMapConsumer(stack, new (_traceMapping().TraceMap)(sourceMap));
    } catch {
      // ignore
    }
  }
  return stack;
}

/***/ })

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
Object.defineProperty(exports, "getCallsite", ({
  enumerable: true,
  get: function () {
    return _getCallsite.default;
  }
}));
var _getCallsite = _interopRequireDefault(__webpack_require__("./src/getCallsite.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
})();

module.exports = __webpack_exports__;
/******/ })()
;