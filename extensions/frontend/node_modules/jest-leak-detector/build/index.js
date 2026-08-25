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
exports["default"] = void 0;
function _nodeUtil() {
  const data = require("node:util");
  _nodeUtil = function () {
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
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
    return data;
  };
  return data;
}
function _getType() {
  const data = require("@jest/get-type");
  _getType = function () {
    return data;
  };
  return data;
}
function _prettyFormat() {
  const data = require("pretty-format");
  _prettyFormat = function () {
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
/// <reference lib="es2021.WeakRef" />

const tick = (0, _nodeUtil().promisify)(setImmediate);
class LeakDetector {
  _isReferenceBeingHeld;
  _shouldGenerateV8HeapSnapshot;
  _finalizationRegistry;
  constructor(value, opt) {
    if ((0, _getType().isPrimitive)(value)) {
      throw new TypeError(['Primitives cannot leak memory.', `You passed a ${typeof value}: <${(0, _prettyFormat().format)(value)}>`].join(' '));
    }

    // When `_finalizationRegistry` is GCed the callback we set will no longer be called,
    this._finalizationRegistry = new FinalizationRegistry(() => {
      this._isReferenceBeingHeld = false;
    });
    this._finalizationRegistry.register(value, undefined);
    this._isReferenceBeingHeld = true;
    this._shouldGenerateV8HeapSnapshot = opt?.shouldGenerateV8HeapSnapshot ?? true;

    // Ensure value is not leaked by the closure created by the "weak" callback.
    value = null;
  }
  async isLeaking() {
    this._runGarbageCollector();

    // wait some ticks to allow GC to run properly, see https://github.com/nodejs/node/issues/34636#issuecomment-669366235
    for (let i = 0; i < 10; i++) {
      await tick();
    }
    if (this._isReferenceBeingHeld) {
      if (this._shouldGenerateV8HeapSnapshot) {
        // Triggering a heap snapshot is more aggressive than just calling `global.gc()`,
        // but it's also quite slow. Only do it if we still think we're leaking.
        // See: https://github.com/nodejs/node/pull/48510#issuecomment-1719289759
        (0, _nodeV().getHeapSnapshot)();
      }
      for (let i = 0; i < 10; i++) {
        await tick();
      }
    }
    return this._isReferenceBeingHeld;
  }
  _runGarbageCollector() {
    const isGarbageCollectorHidden = globalThis.gc == null;

    // GC is usually hidden, so we have to expose it before running.
    (0, _nodeV().setFlagsFromString)('--expose-gc');
    (0, _nodeVm().runInNewContext)('gc')();

    // The GC was not initially exposed, so let's hide it again.
    if (isGarbageCollectorHidden) {
      (0, _nodeV().setFlagsFromString)('--no-expose-gc');
    }
  }
}
exports["default"] = LeakDetector;
})();

module.exports = __webpack_exports__;
/******/ })()
;