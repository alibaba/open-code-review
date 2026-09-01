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
exports.replacePathSepForRegex = exports.escapeStrForRegex = exports.escapePathForRegex = void 0;
var _nodePath = require("node:path");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const escapePathForRegex = dir => {
  if (_nodePath.sep === '\\') {
    // Replace "\" with "/" so it's not escaped by escapeStrForRegex.
    // replacePathSepForRegex will convert it back.
    dir = dir.replaceAll('\\', '/');
  }
  return replacePathSepForRegex(escapeStrForRegex(dir));
};
exports.escapePathForRegex = escapePathForRegex;
const escapeStrForRegex = string => string.replaceAll(/[$()*+.?[\\\]^{|}]/g, '\\$&');
exports.escapeStrForRegex = escapeStrForRegex;
const replacePathSepForRegex = string => {
  if (_nodePath.sep === '\\') {
    return string.replaceAll(/(\/|(.)?\\(?![$()*+.?[\\\]^{|}]))/g, (_match, _, p2) => p2 && p2 !== '\\' ? `${p2}\\\\` : '\\\\');
  }
  return string;
};
exports.replacePathSepForRegex = replacePathSepForRegex;
})();

module.exports = __webpack_exports__;
/******/ })()
;