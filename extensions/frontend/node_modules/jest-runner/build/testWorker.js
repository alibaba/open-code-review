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

/***/ "./src/runTest.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = runTest;
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
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
function sourcemapSupport() {
  const data = _interopRequireWildcard(require("source-map-support"));
  sourcemapSupport = function () {
    return data;
  };
  return data;
}
function _console() {
  const data = require("@jest/console");
  _console = function () {
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
function docblock() {
  const data = _interopRequireWildcard(require("jest-docblock"));
  docblock = function () {
    return data;
  };
  return data;
}
function _jestLeakDetector() {
  const data = _interopRequireDefault(require("jest-leak-detector"));
  _jestLeakDetector = function () {
    return data;
  };
  return data;
}
function _jestMessageUtil() {
  const data = require("jest-message-util");
  _jestMessageUtil = function () {
    return data;
  };
  return data;
}
function _jestResolve() {
  const data = require("jest-resolve");
  _jestResolve = function () {
    return data;
  };
  return data;
}
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

// eslint-disable-next-line @typescript-eslint/consistent-type-imports

function freezeConsole(testConsole, config) {
  // @ts-expect-error: `_log` is `private` - we should figure out some proper API here
  testConsole._log = function fakeConsolePush(_type, message) {
    const error = new (_jestUtil().ErrorWithStack)(`${_chalk().default.red(`${_chalk().default.bold('Cannot log after tests are done.')} Did you forget to wait for something async in your test?`)}\nAttempted to log "${message}".`, fakeConsolePush);
    const formattedError = (0, _jestMessageUtil().formatExecError)(error, config, {
      noStackTrace: false
    }, undefined, true);
    process.stderr.write(`\n${formattedError}\n`);
    process.exitCode = 1;
  };
}

// Keeping the core of "runTest" as a separate function (as "runTestInternal")
// is key to be able to detect memory leaks. Since all variables are local to
// the function, when "runTestInternal" finishes its execution, they can all be
// freed, UNLESS something else is leaking them (and that's why we can detect
// the leak!).
//
// If we had all the code in a single function, we should manually nullify all
// references to verify if there is a leak, which is not maintainable and error
// prone. That's why "runTestInternal" CANNOT be inlined inside "runTest".
async function runTestInternal(path, globalConfig, projectConfig, resolver, context, sendMessageToJest) {
  const testSource = fs().readFileSync(path, 'utf8');
  const docblockPragmas = docblock().parse(docblock().extract(testSource));
  const customEnvironment = docblockPragmas['jest-environment'];
  const loadTestEnvironmentStart = Date.now();
  let testEnvironment = projectConfig.testEnvironment;
  if (customEnvironment) {
    if (Array.isArray(customEnvironment)) {
      throw new TypeError(`You can only define a single test environment through docblocks, got "${customEnvironment.join(', ')}"`);
    }
    testEnvironment = (0, _jestResolve().resolveTestEnvironment)({
      ...projectConfig,
      // we wanna avoid webpack trying to be clever
      requireResolveFunction: module => require.resolve(module),
      testEnvironment: customEnvironment
    });
  }
  const cacheFS = new Map([[path, testSource]]);
  const transformer = await (0, _transform().createScriptTransformer)(projectConfig, cacheFS);
  const TestEnvironment = await transformer.requireAndTranspileModule(testEnvironment);
  const testFramework = await transformer.requireAndTranspileModule(process.env.JEST_JASMINE === '1' ? require.resolve('jest-jasmine2') : projectConfig.testRunner);
  const Runtime = (0, _jestUtil().interopRequireDefault)(projectConfig.runtime ? require(projectConfig.runtime) : require('jest-runtime')).default;
  const consoleOut = globalConfig.useStderr ? process.stderr : process.stdout;
  const consoleFormatter = (type, message) => (0, _console().getConsoleOutput)(
  // 4 = the console call is buried 4 stack frames deep
  _console().BufferedConsole.write([], type, message, 4), projectConfig, globalConfig);
  let testConsole;
  if (projectConfig.silent ?? globalConfig.silent) {
    testConsole = new (_console().NullConsole)(consoleOut, consoleOut, consoleFormatter);
  } else if (projectConfig.verbose ?? globalConfig.verbose) {
    testConsole = new (_console().CustomConsole)(consoleOut, consoleOut, consoleFormatter);
  } else {
    testConsole = new (_console().BufferedConsole)();
  }
  let extraTestEnvironmentOptions;
  const docblockEnvironmentOptions = docblockPragmas['jest-environment-options'];
  if (typeof docblockEnvironmentOptions === 'string') {
    extraTestEnvironmentOptions = JSON.parse(docblockEnvironmentOptions);
  }
  const environment = new TestEnvironment({
    globalConfig,
    projectConfig: extraTestEnvironmentOptions ? {
      ...projectConfig,
      testEnvironmentOptions: {
        ...projectConfig.testEnvironmentOptions,
        ...extraTestEnvironmentOptions
      }
    } : projectConfig
  }, {
    console: testConsole,
    docblockPragmas,
    testPath: path
  });
  const loadTestEnvironmentEnd = Date.now();
  if (typeof environment.getVmContext !== 'function') {
    console.error(`Test environment found at "${testEnvironment}" does not export a "getVmContext" method, which is mandatory from Jest 27. This method is a replacement for "runScript".`);
    process.exit(1);
  }
  const leakDetector = projectConfig.detectLeaks ? new (_jestLeakDetector().default)(environment) : null;
  (0, _jestUtil().setGlobal)(environment.global, 'console', testConsole, 'retain');
  const runtime = new Runtime(projectConfig, environment, resolver, transformer, cacheFS, {
    changedFiles: context.changedFiles,
    collectCoverage: globalConfig.collectCoverage,
    collectCoverageFrom: globalConfig.collectCoverageFrom,
    coverageProvider: globalConfig.coverageProvider,
    globalRootDir: globalConfig.rootDir,
    sourcesRelatedToTestsInChangedFiles: context.sourcesRelatedToTestsInChangedFiles
  }, path, globalConfig);
  let isTornDown = false;
  const tearDownEnv = async () => {
    if (!isTornDown) {
      runtime.teardown();

      // source-map-support keeps memory leftovers in `Error.prepareStackTrace`
      (0, _nodeVm().runInContext)("Error.prepareStackTrace = () => '';", environment.getVmContext());
      sourcemapSupport().resetRetrieveHandlers();
      try {
        await environment.teardown();
      } finally {
        isTornDown = true;
      }
    }
  };
  const start = Date.now();
  const setupFilesStart = Date.now();
  for (const path of projectConfig.setupFiles) {
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
  const setupFilesEnd = Date.now();
  const sourcemapOptions = {
    environment: 'node',
    handleUncaughtExceptions: false,
    retrieveSourceMap: source => {
      const sourceMapSource = runtime.getSourceMaps()?.get(source);
      if (sourceMapSource) {
        try {
          return {
            map: JSON.parse(fs().readFileSync(sourceMapSource, 'utf8')),
            url: source
          };
        } catch {}
      }
      return null;
    }
  };

  // For tests
  runtime.requireInternalModule(require.resolve('source-map-support')).install(sourcemapOptions);

  // For runtime errors
  sourcemapSupport().install(sourcemapOptions);
  if (environment.global && environment.global.process && environment.global.process.exit) {
    const realExit = environment.global.process.exit;
    environment.global.process.exit = function exit(...args) {
      const error = new (_jestUtil().ErrorWithStack)(`process.exit called with "${args.join(', ')}"`, exit);
      const formattedError = (0, _jestMessageUtil().formatExecError)(error, projectConfig, {
        noStackTrace: false
      }, undefined, true);
      process.stderr.write(formattedError);
      return realExit(...args);
    };
  }

  // if we don't have `getVmContext` on the env skip coverage
  const collectV8Coverage = globalConfig.collectCoverage && globalConfig.coverageProvider === 'v8' && typeof environment.getVmContext === 'function';

  // Node's error-message stack size is limited at 10, but it's pretty useful
  // to see more than that when a test fails.
  Error.stackTraceLimit = 100;
  try {
    await environment.setup();
    let result;
    try {
      if (collectV8Coverage) {
        await runtime.collectV8Coverage();
      }
      result = await testFramework(globalConfig, projectConfig, environment, runtime, path, sendMessageToJest);
    } catch (error) {
      // Access all stacks before uninstalling sourcemaps
      let e = error;
      while (typeof e === 'object' && e !== null && 'stack' in e) {
        // eslint-disable-next-line @typescript-eslint/no-unused-expressions
        e.stack;
        e = e?.cause;
      }
      throw error;
    } finally {
      if (collectV8Coverage) {
        await runtime.stopCollectingV8Coverage();
      }
    }
    freezeConsole(testConsole, projectConfig);
    const testCount = result.numPassingTests + result.numFailingTests + result.numPendingTests + result.numTodoTests;
    const end = Date.now();
    const testRuntime = end - start;
    result.perfStats = {
      ...result.perfStats,
      end,
      loadTestEnvironmentEnd,
      loadTestEnvironmentStart,
      runtime: testRuntime,
      setupFilesEnd,
      setupFilesStart,
      slow: testRuntime / 1000 > projectConfig.slowTestThreshold,
      start
    };
    result.testFilePath = path;
    result.console = testConsole.getBuffer();
    result.skipped = testCount === result.numPendingTests;
    result.displayName = projectConfig.displayName;
    const coverage = runtime.getAllCoverageInfoCopy();
    if (coverage) {
      const coverageKeys = Object.keys(coverage);
      if (coverageKeys.length > 0) {
        result.coverage = coverage;
      }
    }
    if (collectV8Coverage) {
      const v8Coverage = runtime.getAllV8CoverageInfoCopy();
      if (v8Coverage && v8Coverage.length > 0) {
        result.v8Coverage = v8Coverage;
      }
    }
    if (globalConfig.logHeapUsage) {
      globalThis.gc?.();
      result.memoryUsage = process.memoryUsage().heapUsed;
    }
    await tearDownEnv();

    // Delay the resolution to allow log messages to be output.
    return await new Promise(resolve => {
      setImmediate(() => resolve({
        leakDetector,
        result
      }));
    });
  } finally {
    await tearDownEnv();
  }
}
async function runTest(path, globalConfig, config, resolver, context, sendMessageToJest) {
  const {
    leakDetector,
    result
  } = await runTestInternal(path, globalConfig, config, resolver, context, sendMessageToJest);
  if (leakDetector) {
    // We wanna allow a tiny but time to pass to allow last-minute cleanup
    await new Promise(resolve => setTimeout(resolve, 100));

    // Resolve leak detector, outside the "runTestInternal" closure.
    result.leaks = await leakDetector.isLeaking();
  } else {
    result.leaks = false;
  }
  return result;
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
exports.setup = setup;
exports.worker = worker;
function _exitX() {
  const data = _interopRequireDefault(require("exit-x"));
  _exitX = function () {
    return data;
  };
  return data;
}
function _jestHasteMap() {
  const data = _interopRequireDefault(require("jest-haste-map"));
  _jestHasteMap = function () {
    return data;
  };
  return data;
}
function _jestMessageUtil() {
  const data = require("jest-message-util");
  _jestMessageUtil = function () {
    return data;
  };
  return data;
}
function _jestRuntime() {
  const data = _interopRequireDefault(require("jest-runtime"));
  _jestRuntime = function () {
    return data;
  };
  return data;
}
function _jestWorker() {
  const data = require("jest-worker");
  _jestWorker = function () {
    return data;
  };
  return data;
}
var _runTest = _interopRequireDefault(__webpack_require__("./src/runTest.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
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
const formatError = error => {
  if (typeof error === 'string') {
    const {
      message,
      stack
    } = (0, _jestMessageUtil().separateMessageFromStack)(error);
    return {
      message,
      stack,
      type: 'Error'
    };
  }
  return {
    code: error.code || undefined,
    message: error.message,
    stack: error.stack,
    type: 'Error'
  };
};
const resolvers = new Map();
const getResolver = config => {
  const resolver = resolvers.get(config.id);
  if (!resolver) {
    throw new Error(`Cannot find resolver for: ${config.id}`);
  }
  return resolver;
};
function setup(setupData) {
  // Module maps that will be needed for the test runs are passed.
  for (const {
    config,
    serializableModuleMap
  } of setupData.serializableResolvers) {
    const moduleMap = _jestHasteMap().default.getStatic(config).getModuleMapFromJSON(serializableModuleMap);
    resolvers.set(config.id, _jestRuntime().default.createResolver(config, moduleMap));
  }
}
const sendMessageToJest = (eventName, args) => {
  (0, _jestWorker().messageParent)([eventName, args]);
};
async function worker({
  config,
  globalConfig,
  path,
  context
}) {
  try {
    return await (0, _runTest.default)(path, globalConfig, config, getResolver(config), {
      ...context,
      changedFiles: context.changedFiles && new Set(context.changedFiles),
      sourcesRelatedToTestsInChangedFiles: context.sourcesRelatedToTestsInChangedFiles && new Set(context.sourcesRelatedToTestsInChangedFiles)
    }, sendMessageToJest);
  } catch (error) {
    throw formatError(error);
  }
}
})();

module.exports = __webpack_exports__;
/******/ })()
;