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

/***/ "./src/legacy-code-todo-rewrite/jestAdapter.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _jestUtil = require("jest-util");
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestNow = globalThis[Symbol.for('jest-native-now')] || globalThis.Date.now;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const FRAMEWORK_INITIALIZER = require.resolve('./jestAdapterInit');
const jestAdapter = async (globalConfig, config, environment, runtime, testPath, sendMessageToJest) => {
  const {
    collectTestsWithoutRunning,
    initialize,
    runAndTransformResultsToJestFormat
  } = runtime.requireInternalModule(FRAMEWORK_INITIALIZER);
  const {
    globals,
    snapshotState
  } = await initialize({
    config,
    environment,
    globalConfig,
    localRequire: runtime.requireModule.bind(runtime),
    parentProcess: process,
    runtime,
    sendMessageToJest,
    setGlobalsForRuntime: runtime.setGlobalsForRuntime.bind(runtime),
    testPath
  });
  if (config.fakeTimers.enableGlobally) {
    if (config.fakeTimers.legacyFakeTimers) {
      // during setup, this cannot be null (and it's fine to explode if it is)
      environment.fakeTimers.useFakeTimers();
    } else {
      environment.fakeTimersModern.useFakeTimers();
    }
  }
  globals.beforeEach(() => {
    if (config.resetModules) {
      runtime.resetModules();
    }
    if (config.clearMocks) {
      runtime.clearAllMocks();
    }
    if (config.resetMocks) {
      runtime.resetAllMocks();
      if (config.fakeTimers.enableGlobally && config.fakeTimers.legacyFakeTimers) {
        // during setup, this cannot be null (and it's fine to explode if it is)
        environment.fakeTimers.useFakeTimers();
      }
    }
    if (config.restoreMocks) {
      runtime.restoreAllMocks();
    }
  });
  const setupAfterEnvStart = jestNow();
  for (const path of config.setupFilesAfterEnv) {
    const esm = runtime.unstable_shouldLoadAsEsm(path);
    if (esm) {
      await runtime.unstable_importModule(path);
    } else {
      const setupFile = runtime.requireModule(path);
      if (typeof setupFile === 'function') {
        await setupFile();
      }
    }
  }
  const setupAfterEnvEnd = jestNow();
  const esm = runtime.unstable_shouldLoadAsEsm(testPath);
  if (esm) {
    await runtime.unstable_importModule(testPath);
  } else {
    runtime.requireModule(testPath);
  }
  if (globalConfig.collectTests) {
    const results = await collectTestsWithoutRunning({
      config,
      testPath
    });
    return (0, _jestUtil.deepCyclicCopy)(results, {
      keepPrototype: false
    });
  }
  const setupAfterEnvPerfStats = {
    setupAfterEnvEnd,
    setupAfterEnvStart
  };
  const results = await runAndTransformResultsToJestFormat({
    config,
    globalConfig,
    setupAfterEnvPerfStats,
    testPath
  });
  _addSnapshotData(results, snapshotState);

  // We need to copy the results object to ensure we don't leaks the prototypes
  // from the VM. Jasmine creates the result objects in the parent process, we
  // should consider doing that for circus as well.
  return (0, _jestUtil.deepCyclicCopy)(results, {
    keepPrototype: false
  });
};
const _addSnapshotData = (results, snapshotState) => {
  for (const {
    fullName,
    status,
    failing
  } of results.testResults) {
    if (status === 'pending' || status === 'failed' || failing && status === 'passed') {
      // If test is skipped or failed, we don't want to mark
      // its snapshots as obsolete.
      // When tests called with test.failing pass, they've thrown an exception,
      // so maintain any snapshots after the error.
      snapshotState.markSnapshotsAsCheckedForTest(fullName);
    }
  }
  const uncheckedCount = snapshotState.getUncheckedCount();
  const uncheckedKeys = snapshotState.getUncheckedKeys();
  if (uncheckedCount) {
    snapshotState.removeUncheckedKeys();
  }
  const status = snapshotState.save();
  results.snapshot.fileDeleted = status.deleted;
  results.snapshot.added = snapshotState.added;
  results.snapshot.matched = snapshotState.matched;
  results.snapshot.unmatched = snapshotState.unmatched;
  results.snapshot.updated = snapshotState.updated;
  results.snapshot.unchecked = status.deleted ? 0 : uncheckedCount;
  // Copy the array to prevent memory leaks
  results.snapshot.uncheckedKeys = [...uncheckedKeys];
};
var _default = exports["default"] = jestAdapter;

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
var _jestAdapter = _interopRequireDefault(__webpack_require__("./src/legacy-code-todo-rewrite/jestAdapter.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
// Allow people to use `jest-circus/runner` as a runner.
var _default = exports["default"] = _jestAdapter.default;
})();

module.exports = __webpack_exports__;
/******/ })()
;