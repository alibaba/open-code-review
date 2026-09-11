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

/***/ "./src/raw-types.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.SnapshotFormat = exports.InitialOptions = exports.FakeTimers = exports.CoverageReporterNames = exports.ChalkForegroundColors = void 0;
function _typebox() {
  const data = require("@sinclair/typebox");
  _typebox = function () {
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

/* eslint-disable sort-keys */

const SnapshotFormat = exports.SnapshotFormat = _typebox().Type.Partial(_typebox().Type.Object({
  callToJSON: _typebox().Type.Boolean(),
  compareKeys: _typebox().Type.Null(),
  escapeRegex: _typebox().Type.Boolean(),
  escapeString: _typebox().Type.Boolean(),
  highlight: _typebox().Type.Boolean(),
  indent: _typebox().Type.Integer({
    minimum: 0
  }),
  maxDepth: _typebox().Type.Integer({
    minimum: 0
  }),
  maxWidth: _typebox().Type.Integer({
    minimum: 0
  }),
  min: _typebox().Type.Boolean(),
  printBasicPrototype: _typebox().Type.Boolean(),
  printFunctionName: _typebox().Type.Boolean(),
  theme: _typebox().Type.Partial(_typebox().Type.Object({
    comment: _typebox().Type.String(),
    content: _typebox().Type.String(),
    prop: _typebox().Type.String(),
    tag: _typebox().Type.String(),
    value: _typebox().Type.String()
  }))
}));
const CoverageProvider = _typebox().Type.Union([_typebox().Type.Literal('babel'), _typebox().Type.Literal('v8')]);
const CoverageThresholdValue = _typebox().Type.Partial(_typebox().Type.Object({
  branches: _typebox().Type.Number({
    minimum: 0,
    maximum: 100
  }),
  functions: _typebox().Type.Number({
    minimum: 0,
    maximum: 100
  }),
  lines: _typebox().Type.Number({
    minimum: 0,
    maximum: 100
  }),
  statements: _typebox().Type.Number({
    minimum: 0,
    maximum: 100
  })
}));
const CoverageThresholdBase = _typebox().Type.Object({
  global: CoverageThresholdValue
}, {
  additionalProperties: CoverageThresholdValue
});
const CoverageThreshold = _typebox().Type.Unsafe(CoverageThresholdBase);

// TODO: add type test that these are all the colors available in chalk.ForegroundColor
const ChalkForegroundColors = exports.ChalkForegroundColors = _typebox().Type.Union([_typebox().Type.Literal('black'), _typebox().Type.Literal('red'), _typebox().Type.Literal('green'), _typebox().Type.Literal('yellow'), _typebox().Type.Literal('blue'), _typebox().Type.Literal('magenta'), _typebox().Type.Literal('cyan'), _typebox().Type.Literal('white'), _typebox().Type.Literal('gray'), _typebox().Type.Literal('grey'), _typebox().Type.Literal('blackBright'), _typebox().Type.Literal('redBright'), _typebox().Type.Literal('greenBright'), _typebox().Type.Literal('yellowBright'), _typebox().Type.Literal('blueBright'), _typebox().Type.Literal('magentaBright'), _typebox().Type.Literal('cyanBright'), _typebox().Type.Literal('whiteBright')]);
const DisplayName = _typebox().Type.Object({
  name: _typebox().Type.String(),
  color: ChalkForegroundColors
});

// TODO: verify these are the names of istanbulReport.ReportOptions
const CoverageReporterNames = exports.CoverageReporterNames = _typebox().Type.Union([_typebox().Type.Literal('clover'), _typebox().Type.Literal('cobertura'), _typebox().Type.Literal('html-spa'), _typebox().Type.Literal('html'), _typebox().Type.Literal('json'), _typebox().Type.Literal('json-summary'), _typebox().Type.Literal('lcov'), _typebox().Type.Literal('lcovonly'), _typebox().Type.Literal('none'), _typebox().Type.Literal('teamcity'), _typebox().Type.Literal('text'), _typebox().Type.Literal('text-lcov'), _typebox().Type.Literal('text-summary')]);
const CoverageReporters = _typebox().Type.Array(_typebox().Type.Union([CoverageReporterNames, _typebox().Type.Tuple([CoverageReporterNames, _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Unknown())])]));
const GlobalFakeTimersConfig = _typebox().Type.Partial(_typebox().Type.Object({
  enableGlobally: _typebox().Type.Boolean({
    description: 'Whether fake timers should be enabled globally for all test files.',
    default: false
  })
}));
const FakeableAPI = _typebox().Type.Union([_typebox().Type.Literal('Date'), _typebox().Type.Literal('hrtime'), _typebox().Type.Literal('nextTick'), _typebox().Type.Literal('performance'), _typebox().Type.Literal('queueMicrotask'), _typebox().Type.Literal('requestAnimationFrame'), _typebox().Type.Literal('cancelAnimationFrame'), _typebox().Type.Literal('requestIdleCallback'), _typebox().Type.Literal('cancelIdleCallback'), _typebox().Type.Literal('setImmediate'), _typebox().Type.Literal('clearImmediate'), _typebox().Type.Literal('setInterval'), _typebox().Type.Literal('clearInterval'), _typebox().Type.Literal('setTimeout'), _typebox().Type.Literal('clearTimeout'), _typebox().Type.Literal('Temporal')]);
const FakeTimersConfig = _typebox().Type.Partial(_typebox().Type.Object({
  advanceTimers: _typebox().Type.Union([_typebox().Type.Boolean(), _typebox().Type.Number({
    minimum: 0
  })], {
    description: 'If set to `true` all timers will be advanced automatically by 20 milliseconds every 20 milliseconds. A custom ' + 'time delta may be provided by passing a number.',
    default: false
  }),
  doNotFake: _typebox().Type.Array(FakeableAPI, {
    description: 'List of names of APIs (e.g. `Date`, `nextTick()`, `setImmediate()`, `setTimeout()`) that should not be faked.' + '\n\nThe default is `[]`, meaning all APIs are faked.',
    default: []
  }),
  now: _typebox().Type.Integer({
    minimum: 0,
    description: 'Sets current system time to be used by fake timers.\n\nThe default is `Date.now()`.'
  }),
  timerLimit: _typebox().Type.Number({
    description: 'The maximum number of recursive timers that will be run when calling `jest.runAllTimers()`.',
    default: 100_000,
    minimum: 0
  }),
  legacyFakeTimers: _typebox().Type.Literal(false, {
    description: 'Use the old fake timers implementation instead of one backed by `@sinonjs/fake-timers`.',
    default: false
  })
}));
const LegacyFakeTimersConfig = _typebox().Type.Partial(_typebox().Type.Object({
  legacyFakeTimers: _typebox().Type.Literal(true, {
    description: 'Use the old fake timers implementation instead of one backed by `@sinonjs/fake-timers`.',
    default: true
  })
}));
const FakeTimers = exports.FakeTimers = _typebox().Type.Intersect([GlobalFakeTimersConfig, _typebox().Type.Union([FakeTimersConfig, LegacyFakeTimersConfig])]);
const HasteConfig = _typebox().Type.Partial(_typebox().Type.Object({
  computeSha1: _typebox().Type.Boolean({
    description: 'Whether to hash files using SHA-1.'
  }),
  defaultPlatform: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Null()], {
    description: 'The platform to use as the default, e.g. `ios`.'
  }),
  forceNodeFilesystemAPI: _typebox().Type.Boolean({
    description: "Whether to force the use of Node's `fs` API when reading files rather than shelling out to `find`."
  }),
  enableSymlinks: _typebox().Type.Boolean({
    description: 'Whether to follow symlinks when crawling for files.' + '\n\tThis options cannot be used in projects which use watchman.' + '\n\tProjects with `watchman` set to true will error if this option is set to true.'
  }),
  hasteImplModulePath: _typebox().Type.String({
    description: 'Path to a custom implementation of Haste.'
  }),
  platforms: _typebox().Type.Array(_typebox().Type.String(), {
    description: "All platforms to target, e.g ['ios', 'android']."
  }),
  throwOnModuleCollision: _typebox().Type.Boolean({
    description: 'Whether to throw an error on module collision.'
  }),
  hasteMapModulePath: _typebox().Type.String({
    description: 'Custom HasteMap module'
  }),
  retainAllFiles: _typebox().Type.Boolean({
    description: 'Whether to retain all files, allowing e.g. search for tests in `node_modules`.'
  })
}));
const InitialOptions = exports.InitialOptions = _typebox().Type.Partial(_typebox().Type.Object({
  automock: _typebox().Type.Boolean(),
  bail: _typebox().Type.Union([_typebox().Type.Boolean(), _typebox().Type.Number()]),
  cache: _typebox().Type.Boolean(),
  cacheDirectory: _typebox().Type.String(),
  ci: _typebox().Type.Boolean(),
  clearMocks: _typebox().Type.Boolean(),
  changedFilesWithAncestor: _typebox().Type.Boolean(),
  changedSince: _typebox().Type.String(),
  collectCoverage: _typebox().Type.Boolean(),
  collectCoverageFrom: _typebox().Type.Array(_typebox().Type.String()),
  collectTests: _typebox().Type.Boolean(),
  coverageDirectory: _typebox().Type.String(),
  coveragePathIgnorePatterns: _typebox().Type.Array(_typebox().Type.String()),
  coverageProvider: CoverageProvider,
  coverageReporters: CoverageReporters,
  coverageThreshold: CoverageThreshold,
  dependencyExtractor: _typebox().Type.String(),
  detectLeaks: _typebox().Type.Boolean(),
  detectOpenHandles: _typebox().Type.Boolean(),
  displayName: _typebox().Type.Union([_typebox().Type.String(), DisplayName]),
  expand: _typebox().Type.Boolean(),
  extensionsToTreatAsEsm: _typebox().Type.Array(_typebox().Type.String()),
  fakeTimers: FakeTimers,
  filter: _typebox().Type.String(),
  findRelatedTests: _typebox().Type.Boolean(),
  forceCoverageMatch: _typebox().Type.Array(_typebox().Type.String()),
  forceExit: _typebox().Type.Boolean(),
  json: _typebox().Type.Boolean(),
  globals: _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Unknown()),
  globalSetup: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Null()]),
  globalTeardown: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Null()]),
  haste: HasteConfig,
  id: _typebox().Type.String(),
  injectGlobals: _typebox().Type.Boolean(),
  reporters: _typebox().Type.Array(_typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Tuple([_typebox().Type.String(), _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Unknown())])])),
  logHeapUsage: _typebox().Type.Boolean(),
  lastCommit: _typebox().Type.Boolean(),
  listTests: _typebox().Type.Boolean(),
  maxConcurrency: _typebox().Type.Integer(),
  maxWorkers: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Integer()]),
  moduleDirectories: _typebox().Type.Array(_typebox().Type.String()),
  moduleFileExtensions: _typebox().Type.Array(_typebox().Type.String()),
  moduleNameMapper: _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Array(_typebox().Type.String())])),
  modulePathIgnorePatterns: _typebox().Type.Array(_typebox().Type.String()),
  modulePaths: _typebox().Type.Array(_typebox().Type.String()),
  noStackTrace: _typebox().Type.Boolean(),
  notify: _typebox().Type.Boolean(),
  notifyMode: _typebox().Type.String(),
  onlyChanged: _typebox().Type.Boolean(),
  onlyFailures: _typebox().Type.Boolean(),
  openHandlesTimeout: _typebox().Type.Number(),
  outputFile: _typebox().Type.String(),
  passWithNoTests: _typebox().Type.Boolean(),
  preset: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Null()]),
  prettierPath: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Null()]),
  projects: _typebox().Type.Array(_typebox().Type.Union([_typebox().Type.String(),
  // TODO: Make sure to type these correctly
  _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Unknown())])),
  randomize: _typebox().Type.Boolean(),
  replname: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Null()]),
  resetMocks: _typebox().Type.Boolean(),
  resetModules: _typebox().Type.Boolean(),
  resolver: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Null()]),
  restoreMocks: _typebox().Type.Boolean(),
  rootDir: _typebox().Type.String(),
  roots: _typebox().Type.Array(_typebox().Type.String()),
  runner: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Tuple([_typebox().Type.String(), _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Unknown())])]),
  runTestsByPath: _typebox().Type.Boolean(),
  runtime: _typebox().Type.String(),
  sandboxInjectedGlobals: _typebox().Type.Array(_typebox().Type.String()),
  setupFiles: _typebox().Type.Array(_typebox().Type.String()),
  setupFilesAfterEnv: _typebox().Type.Array(_typebox().Type.String()),
  showSeed: _typebox().Type.Boolean(),
  silent: _typebox().Type.Boolean(),
  skipFilter: _typebox().Type.Boolean(),
  skipNodeResolution: _typebox().Type.Boolean(),
  slowTestThreshold: _typebox().Type.Number(),
  snapshotResolver: _typebox().Type.String(),
  snapshotSerializers: _typebox().Type.Array(_typebox().Type.String()),
  snapshotFormat: SnapshotFormat,
  errorOnDeprecated: _typebox().Type.Boolean(),
  testEnvironment: _typebox().Type.String(),
  testEnvironmentOptions: _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Unknown()),
  testFailureExitCode: _typebox().Type.Integer(),
  testLocationInResults: _typebox().Type.Boolean(),
  testMatch: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Array(_typebox().Type.String())]),
  testNamePattern: _typebox().Type.String(),
  testPathIgnorePatterns: _typebox().Type.Array(_typebox().Type.String()),
  testRegex: _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Array(_typebox().Type.String())]),
  testResultsProcessor: _typebox().Type.String(),
  testRunner: _typebox().Type.String(),
  testSequencer: _typebox().Type.String(),
  testTimeout: _typebox().Type.Number(),
  transform: _typebox().Type.Record(_typebox().Type.String(), _typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Tuple([_typebox().Type.String(), _typebox().Type.Unknown()])])),
  transformIgnorePatterns: _typebox().Type.Array(_typebox().Type.String()),
  watchPathIgnorePatterns: _typebox().Type.Array(_typebox().Type.String()),
  unmockedModulePathPatterns: _typebox().Type.Array(_typebox().Type.String()),
  updateSnapshot: _typebox().Type.Boolean(),
  useStderr: _typebox().Type.Boolean(),
  verbose: _typebox().Type.Boolean(),
  waitForUnhandledRejections: _typebox().Type.Boolean(),
  watch: _typebox().Type.Boolean(),
  watchAll: _typebox().Type.Boolean(),
  watchman: _typebox().Type.Boolean(),
  watchPlugins: _typebox().Type.Array(_typebox().Type.Union([_typebox().Type.String(), _typebox().Type.Tuple([_typebox().Type.String(), _typebox().Type.Unknown()])])),
  workerIdleMemoryLimit: _typebox().Type.Union([_typebox().Type.Number(), _typebox().Type.String()]),
  workerThreads: _typebox().Type.Boolean(),
  workerGracefulExitTimeout: _typebox().Type.Number({
    description: 'Timeout in milliseconds for worker processes to exit gracefully after tests complete. Workers that do not exit in time are force-killed. Default: 500.'
  })
}));

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
exports.SnapshotFormat = exports.InitialOptions = exports.FakeTimers = void 0;
var types = _interopRequireWildcard(__webpack_require__("./src/raw-types.ts"));
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const SnapshotFormat = exports.SnapshotFormat = types.SnapshotFormat;
const InitialOptions = exports.InitialOptions = types.InitialOptions;
const FakeTimers = exports.FakeTimers = types.FakeTimers;
})();

module.exports = __webpack_exports__;
/******/ })()
;