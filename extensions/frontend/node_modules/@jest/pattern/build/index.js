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

/***/ "./src/TestPathPatterns.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.TestPathPatternsExecutor = exports.TestPathPatterns = void 0;
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
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class TestPathPatterns {
  constructor(patterns) {
    this.patterns = patterns;
  }

  /**
   * Return true if there are any patterns.
   */
  isSet() {
    return this.patterns.length > 0;
  }

  /**
   * Return true if the patterns are valid.
   */
  isValid() {
    return this.toExecutor({
      // isValid() doesn't require rootDir to be accurate, so just
      // specify a dummy rootDir here
      rootDir: '/'
    }).isValid();
  }

  /**
   * Return a human-friendly version of the pattern regex.
   */
  toPretty() {
    return this.patterns.join('|');
  }

  /**
   * Return a TestPathPatternsExecutor that can execute the patterns.
   */
  toExecutor(options) {
    return new TestPathPatternsExecutor(this, options);
  }

  /** For jest serializers */
  toJSON() {
    return {
      patterns: this.patterns,
      type: 'TestPathPatterns'
    };
  }
}
exports.TestPathPatterns = TestPathPatterns;
class TestPathPatternsExecutor {
  constructor(patterns, options) {
    this.patterns = patterns;
    this.options = options;
  }
  toRegex(s) {
    return new RegExp(s, 'i');
  }

  /**
   * Return true if there are any patterns.
   */
  isSet() {
    return this.patterns.isSet();
  }

  /**
   * Return true if the patterns are valid.
   */
  isValid() {
    try {
      for (const p of this.patterns.patterns) {
        this.toRegex(p);
      }
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Return true if the given ABSOLUTE path matches the patterns.
   *
   * Throws an error if the patterns form an invalid regex (see `validate`).
   */
  isMatch(absPath) {
    const relPath = path().relative(this.options.rootDir || '/', absPath);
    if (this.patterns.patterns.length === 0) {
      return true;
    }
    for (const p of this.patterns.patterns) {
      const pathToTest = path().isAbsolute(p) ? absPath : relPath;

      // special case: ./foo.spec.js (and .\foo.spec.js on Windows) should
      // match /^foo.spec.js/ after stripping root dir
      let regexStr = p.replace(/^\.\//, '^');
      if (path().sep === '\\') {
        regexStr = regexStr.replace(/^\.\\/, '^');
      }
      regexStr = (0, _jestRegexUtil().replacePathSepForRegex)(regexStr);
      if (this.toRegex(regexStr).test(pathToTest)) {
        return true;
      }
      if (this.toRegex(regexStr).test(absPath)) {
        return true;
      }
    }
    return false;
  }

  /**
   * Return a human-friendly version of the pattern regex.
   */
  toPretty() {
    return this.patterns.toPretty();
  }
}
exports.TestPathPatternsExecutor = TestPathPatternsExecutor;

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
Object.defineProperty(exports, "TestPathPatterns", ({
  enumerable: true,
  get: function () {
    return _TestPathPatterns.TestPathPatterns;
  }
}));
Object.defineProperty(exports, "TestPathPatternsExecutor", ({
  enumerable: true,
  get: function () {
    return _TestPathPatterns.TestPathPatternsExecutor;
  }
}));
var _TestPathPatterns = __webpack_require__("./src/TestPathPatterns.ts");
})();

module.exports = __webpack_exports__;
/******/ })()
;