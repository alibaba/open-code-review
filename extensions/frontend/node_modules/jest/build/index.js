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
var __webpack_exports__ = {};
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
Object.defineProperty(exports, "SearchSource", ({
  enumerable: true,
  get: function () {
    return _core().SearchSource;
  }
}));
Object.defineProperty(exports, "buildArgv", ({
  enumerable: true,
  get: function () {
    return _jestCli().buildArgv;
  }
}));
Object.defineProperty(exports, "createTestScheduler", ({
  enumerable: true,
  get: function () {
    return _core().createTestScheduler;
  }
}));
Object.defineProperty(exports, "defineConfig", ({
  enumerable: true,
  get: function () {
    return _jestConfig().defineConfig;
  }
}));
Object.defineProperty(exports, "getVersion", ({
  enumerable: true,
  get: function () {
    return _core().getVersion;
  }
}));
Object.defineProperty(exports, "mergeConfig", ({
  enumerable: true,
  get: function () {
    return _jestConfig().mergeConfig;
  }
}));
Object.defineProperty(exports, "run", ({
  enumerable: true,
  get: function () {
    return _jestCli().run;
  }
}));
Object.defineProperty(exports, "runCLI", ({
  enumerable: true,
  get: function () {
    return _core().runCLI;
  }
}));
function _core() {
  const data = require("@jest/core");
  _core = function () {
    return data;
  };
  return data;
}
function _jestCli() {
  const data = require("jest-cli");
  _jestCli = function () {
    return data;
  };
  return data;
}
function _jestConfig() {
  const data = require("jest-config");
  _jestConfig = function () {
    return data;
  };
  return data;
}
})();

module.exports = __webpack_exports__;
/******/ })()
;