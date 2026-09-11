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

/***/ "./src/formatTestResults.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = formatTestResults;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const formatTestResult = (testResult, codeCoverageFormatter, reporter) => {
  if (testResult.testExecError) {
    const now = Date.now();
    return {
      assertionResults: testResult.testResults,
      coverage: {},
      endTime: now,
      message: testResult.failureMessage ?? testResult.testExecError.message,
      name: testResult.testFilePath,
      startTime: now,
      status: 'failed',
      summary: ''
    };
  }
  if (testResult.skipped) {
    const now = Date.now();
    return {
      assertionResults: testResult.testResults,
      coverage: {},
      endTime: now,
      message: testResult.failureMessage ?? '',
      name: testResult.testFilePath,
      startTime: now,
      status: 'skipped',
      summary: ''
    };
  }
  const allTestsExecuted = testResult.numPendingTests === 0;
  const allTestsPassed = testResult.numFailingTests === 0;
  return {
    assertionResults: testResult.testResults,
    coverage: codeCoverageFormatter == null ? testResult.coverage : codeCoverageFormatter(testResult.coverage, reporter),
    endTime: testResult.perfStats.end,
    message: testResult.failureMessage ?? '',
    name: testResult.testFilePath,
    startTime: testResult.perfStats.start,
    status: allTestsPassed ? allTestsExecuted ? 'passed' : 'focused' : 'failed',
    summary: ''
  };
};
function formatTestResults(results, codeCoverageFormatter, reporter) {
  const testResults = results.testResults.map(testResult => formatTestResult(testResult, codeCoverageFormatter, reporter));
  return {
    ...results,
    testResults
  };
}

/***/ },

/***/ "./src/helpers.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.makeEmptyAggregatedTestResult = exports.createEmptyTestResult = exports.buildFailureTestResult = exports.addResult = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const makeEmptyAggregatedTestResult = () => ({
  numFailedTestSuites: 0,
  numFailedTests: 0,
  numPassedTestSuites: 0,
  numPassedTests: 0,
  numPendingTestSuites: 0,
  numPendingTests: 0,
  numRuntimeErrorTestSuites: 0,
  numTodoTests: 0,
  numTotalTestSuites: 0,
  numTotalTests: 0,
  openHandles: [],
  snapshot: {
    added: 0,
    didUpdate: false,
    // is set only after the full run
    failure: false,
    filesAdded: 0,
    // combines individual test results + removed files after the full run
    filesRemoved: 0,
    filesRemovedList: [],
    filesUnmatched: 0,
    filesUpdated: 0,
    matched: 0,
    total: 0,
    unchecked: 0,
    uncheckedKeysByFile: [],
    unmatched: 0,
    updated: 0
  },
  startTime: 0,
  success: true,
  testResults: [],
  wasInterrupted: false
});
exports.makeEmptyAggregatedTestResult = makeEmptyAggregatedTestResult;
const buildFailureTestResult = (testPath, err) => ({
  console: undefined,
  displayName: undefined,
  failureMessage: null,
  leaks: false,
  numFailingTests: 0,
  numPassingTests: 0,
  numPendingTests: 0,
  numTodoTests: 0,
  openHandles: [],
  perfStats: {
    end: 0,
    loadTestEnvironmentEnd: 0,
    loadTestEnvironmentStart: 0,
    runtime: 0,
    setupAfterEnvEnd: 0,
    setupAfterEnvStart: 0,
    setupFilesEnd: 0,
    setupFilesStart: 0,
    slow: false,
    start: 0
  },
  skipped: false,
  snapshot: {
    added: 0,
    fileDeleted: false,
    matched: 0,
    unchecked: 0,
    uncheckedKeys: [],
    unmatched: 0,
    updated: 0
  },
  testExecError: err,
  testFilePath: testPath,
  testResults: []
});

// Add individual test result to an aggregated test result
exports.buildFailureTestResult = buildFailureTestResult;
const addResult = (aggregatedResults, testResult) => {
  // `todos` are new as of Jest 24, and not all runners return it.
  // Set it to `0` to avoid `NaN`
  if (!testResult.numTodoTests) {
    testResult.numTodoTests = 0;
  }
  aggregatedResults.testResults.push(testResult);
  aggregatedResults.numTotalTests += testResult.numPassingTests + testResult.numFailingTests + testResult.numPendingTests + testResult.numTodoTests;
  aggregatedResults.numFailedTests += testResult.numFailingTests;
  aggregatedResults.numPassedTests += testResult.numPassingTests;
  aggregatedResults.numPendingTests += testResult.numPendingTests;
  aggregatedResults.numTodoTests += testResult.numTodoTests;
  if (testResult.testExecError) {
    aggregatedResults.numRuntimeErrorTestSuites++;
  }
  if (testResult.skipped) {
    aggregatedResults.numPendingTestSuites++;
  } else if (testResult.numFailingTests > 0 || testResult.testExecError) {
    aggregatedResults.numFailedTestSuites++;
  } else {
    aggregatedResults.numPassedTestSuites++;
  }

  // Snapshot data
  if (testResult.snapshot.added) {
    aggregatedResults.snapshot.filesAdded++;
  }
  if (testResult.snapshot.fileDeleted) {
    aggregatedResults.snapshot.filesRemoved++;
  }
  if (testResult.snapshot.unmatched) {
    aggregatedResults.snapshot.filesUnmatched++;
  }
  if (testResult.snapshot.updated) {
    aggregatedResults.snapshot.filesUpdated++;
  }
  aggregatedResults.snapshot.added += testResult.snapshot.added;
  aggregatedResults.snapshot.matched += testResult.snapshot.matched;
  aggregatedResults.snapshot.unchecked += testResult.snapshot.unchecked;
  if (testResult.snapshot.uncheckedKeys != null && testResult.snapshot.uncheckedKeys.length > 0) {
    aggregatedResults.snapshot.uncheckedKeysByFile.push({
      filePath: testResult.testFilePath,
      keys: testResult.snapshot.uncheckedKeys
    });
  }
  aggregatedResults.snapshot.unmatched += testResult.snapshot.unmatched;
  aggregatedResults.snapshot.updated += testResult.snapshot.updated;
  aggregatedResults.snapshot.total += testResult.snapshot.added + testResult.snapshot.matched + testResult.snapshot.unmatched + testResult.snapshot.updated;
};
exports.addResult = addResult;
const createEmptyTestResult = () => ({
  leaks: false,
  // That's legacy code, just adding it as needed for typing
  numFailingTests: 0,
  numPassingTests: 0,
  numPendingTests: 0,
  numTodoTests: 0,
  openHandles: [],
  perfStats: {
    end: 0,
    loadTestEnvironmentEnd: 0,
    loadTestEnvironmentStart: 0,
    runtime: 0,
    setupAfterEnvEnd: 0,
    setupAfterEnvStart: 0,
    setupFilesEnd: 0,
    setupFilesStart: 0,
    slow: false,
    start: 0
  },
  skipped: false,
  snapshot: {
    added: 0,
    fileDeleted: false,
    matched: 0,
    unchecked: 0,
    uncheckedKeys: [],
    unmatched: 0,
    updated: 0
  },
  testFilePath: '',
  testResults: []
});
exports.createEmptyTestResult = createEmptyTestResult;

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
Object.defineProperty(exports, "addResult", ({
  enumerable: true,
  get: function () {
    return _helpers.addResult;
  }
}));
Object.defineProperty(exports, "buildFailureTestResult", ({
  enumerable: true,
  get: function () {
    return _helpers.buildFailureTestResult;
  }
}));
Object.defineProperty(exports, "createEmptyTestResult", ({
  enumerable: true,
  get: function () {
    return _helpers.createEmptyTestResult;
  }
}));
Object.defineProperty(exports, "formatTestResults", ({
  enumerable: true,
  get: function () {
    return _formatTestResults.default;
  }
}));
Object.defineProperty(exports, "makeEmptyAggregatedTestResult", ({
  enumerable: true,
  get: function () {
    return _helpers.makeEmptyAggregatedTestResult;
  }
}));
var _formatTestResults = _interopRequireDefault(__webpack_require__("./src/formatTestResults.ts"));
var _helpers = __webpack_require__("./src/helpers.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
})();

module.exports = __webpack_exports__;
/******/ })()
;