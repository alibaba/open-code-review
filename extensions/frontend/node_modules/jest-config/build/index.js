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

/***/ "./src/Defaults.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _nodePath() {
  const data = require("node:path");
  _nodePath = function () {
    return data;
  };
  return data;
}
function _ciInfo() {
  const data = require("ci-info");
  _ciInfo = function () {
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
var _constants = __webpack_require__("./src/constants.ts");
var _getCacheDirectory = _interopRequireDefault(__webpack_require__("./src/getCacheDirectory.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const NODE_MODULES_REGEXP = (0, _jestRegexUtil().replacePathSepForRegex)(_constants.NODE_MODULES);
const defaultOptions = {
  automock: false,
  bail: 0,
  cache: true,
  cacheDirectory: (0, _getCacheDirectory.default)(),
  changedFilesWithAncestor: false,
  ci: _ciInfo().isCI,
  clearMocks: false,
  collectCoverage: false,
  coveragePathIgnorePatterns: [NODE_MODULES_REGEXP],
  coverageProvider: 'babel',
  coverageReporters: ['json', 'text', 'lcov', 'clover'],
  detectLeaks: false,
  detectOpenHandles: false,
  errorOnDeprecated: false,
  expand: false,
  extensionsToTreatAsEsm: [],
  fakeTimers: {
    enableGlobally: false
  },
  forceCoverageMatch: [],
  globals: {},
  haste: {
    computeSha1: false,
    enableSymlinks: false,
    forceNodeFilesystemAPI: true,
    throwOnModuleCollision: false
  },
  injectGlobals: true,
  listTests: false,
  maxConcurrency: 5,
  maxWorkers: '50%',
  moduleDirectories: ['node_modules'],
  moduleFileExtensions: ['js', 'mjs', 'cjs', 'jsx', 'ts', 'mts', 'cts', 'tsx', 'json', 'node'],
  moduleNameMapper: {},
  modulePathIgnorePatterns: [],
  noStackTrace: false,
  notify: false,
  notifyMode: 'failure-change',
  openHandlesTimeout: 1000,
  passWithNoTests: false,
  prettierPath: 'prettier',
  resetMocks: false,
  resetModules: false,
  restoreMocks: false,
  roots: ['<rootDir>'],
  runTestsByPath: false,
  runner: 'jest-runner',
  setupFiles: [],
  setupFilesAfterEnv: [],
  skipFilter: false,
  slowTestThreshold: 5,
  snapshotFormat: {
    escapeString: false,
    printBasicPrototype: false
  },
  snapshotSerializers: [],
  testEnvironment: 'jest-environment-node',
  testEnvironmentOptions: {},
  testFailureExitCode: 1,
  testLocationInResults: false,
  testMatch: ['**/__tests__/**/*.?([mc])[jt]s?(x)', '**/?(*.)+(spec|test).?([mc])[jt]s?(x)'],
  testPathIgnorePatterns: [NODE_MODULES_REGEXP],
  testRegex: [],
  testRunner: 'jest-circus/runner',
  testSequencer: '@jest/test-sequencer',
  transformIgnorePatterns: [NODE_MODULES_REGEXP, `\\.pnp\\.[^\\${_nodePath().sep}]+$`],
  useStderr: false,
  waitForUnhandledRejections: false,
  watch: false,
  watchPathIgnorePatterns: [],
  watchman: true,
  workerGracefulExitTimeout: 500,
  workerThreads: false
};
var _default = exports["default"] = defaultOptions;

/***/ },

/***/ "./src/Deprecated.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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

function formatDeprecation(message) {
  const lines = [message.replaceAll(/\*(.+?)\*/g, (_, s) => _chalk().default.bold(`"${s}"`)), '', 'Please update your configuration.'];
  return lines.map(s => `  ${s}`).join('\n');
}
const deprecatedOptions = {
  browser: () => `  Option ${_chalk().default.bold('"browser"')} has been deprecated. Please install "browser-resolve" and use the "resolver" option in Jest configuration as shown in the documentation: https://jestjs.io/docs/configuration#resolver-string`,
  collectCoverageOnlyFrom: _options => `  Option ${_chalk().default.bold('"collectCoverageOnlyFrom"')} was replaced by ${_chalk().default.bold('"collectCoverageFrom"')}.

    Please update your configuration.`,
  extraGlobals: _options => `  Option ${_chalk().default.bold('"extraGlobals"')} was replaced by ${_chalk().default.bold('"sandboxInjectedGlobals"')}.

  Please update your configuration.`,
  init: () => `  Option ${_chalk().default.bold('"init"')} has been deprecated. Please use "create-jest" package as shown in the documentation: https://jestjs.io/docs/getting-started#generate-a-basic-configuration-file`,
  moduleLoader: _options => `  Option ${_chalk().default.bold('"moduleLoader"')} was replaced by ${_chalk().default.bold('"runtime"')}.

  Please update your configuration.`,
  preprocessorIgnorePatterns: _options => `  Option ${_chalk().default.bold('"preprocessorIgnorePatterns"')} was replaced by ${_chalk().default.bold('"transformIgnorePatterns"')}, which support multiple preprocessors.

  Please update your configuration.`,
  scriptPreprocessor: _options => `  Option ${_chalk().default.bold('"scriptPreprocessor"')} was replaced by ${_chalk().default.bold('"transform"')}, which support multiple preprocessors.

  Please update your configuration.`,
  setupTestFrameworkScriptFile: _options => `  Option ${_chalk().default.bold('"setupTestFrameworkScriptFile"')} was replaced by configuration ${_chalk().default.bold('"setupFilesAfterEnv"')}, which supports multiple paths.

  Please update your configuration.`,
  testPathDirs: _options => `  Option ${_chalk().default.bold('"testPathDirs"')} was replaced by ${_chalk().default.bold('"roots"')}.

  Please update your configuration.
  `,
  testPathPattern: () => formatDeprecation('Option *testPathPattern* was replaced by *--testPathPatterns*. *--testPathPatterns* is only available as a command-line option.'),
  testURL: _options => `  Option ${_chalk().default.bold('"testURL"')} was replaced by passing the URL via ${_chalk().default.bold('"testEnvironmentOptions.url"')}.

  Please update your configuration.`,
  timers: _options => `  Option ${_chalk().default.bold('"timers"')} was replaced by ${_chalk().default.bold('"fakeTimers"')}.

  Please update your configuration.`
};
var _default = exports["default"] = deprecatedOptions;

/***/ },

/***/ "./src/Descriptions.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const descriptions = {
  automock: 'All imported modules in your tests should be mocked automatically',
  bail: 'Stop running tests after `n` failures',
  cacheDirectory: 'The directory where Jest should store its cached dependency information',
  clearMocks: 'Automatically clear mock calls, instances, contexts and results before every test',
  collectCoverage: 'Indicates whether the coverage information should be collected while executing the test',
  collectCoverageFrom: 'An array of glob patterns indicating a set of files for which coverage information should be collected',
  coverageDirectory: 'The directory where Jest should output its coverage files',
  coveragePathIgnorePatterns: 'An array of regexp pattern strings used to skip coverage collection',
  coverageProvider: 'Indicates which provider should be used to instrument code for coverage',
  coverageReporters: 'A list of reporter names that Jest uses when writing coverage reports',
  coverageThreshold: 'An object that configures minimum threshold enforcement for coverage results',
  dependencyExtractor: 'A path to a custom dependency extractor',
  errorOnDeprecated: 'Make calling deprecated APIs throw helpful error messages',
  fakeTimers: 'The default configuration for fake timers',
  forceCoverageMatch: 'Force coverage collection from ignored files using an array of glob patterns',
  globalSetup: 'A path to a module which exports an async function that is triggered once before all test suites',
  globalTeardown: 'A path to a module which exports an async function that is triggered once after all test suites',
  globals: 'A set of global variables that need to be available in all test environments',
  maxWorkers: 'The maximum amount of workers used to run your tests. Can be specified as % or a number. E.g. maxWorkers: 10% will use 10% of your CPU amount + 1 as the maximum worker number. maxWorkers: 2 will use a maximum of 2 workers.',
  moduleDirectories: "An array of directory names to be searched recursively up from the requiring module's location",
  moduleFileExtensions: 'An array of file extensions your modules use',
  moduleNameMapper: 'A map from regular expressions to module names or to arrays of module names that allow to stub out resources with a single module',
  modulePathIgnorePatterns: "An array of regexp pattern strings, matched against all module paths before considered 'visible' to the module loader",
  notify: 'Activates notifications for test results',
  notifyMode: 'An enum that specifies notification mode. Requires { notify: true }',
  preset: "A preset that is used as a base for Jest's configuration",
  projects: 'Run tests from one or more projects',
  reporters: 'Use this configuration option to add custom reporters to Jest',
  resetMocks: 'Automatically reset mock state before every test',
  resetModules: 'Reset the module registry before running each individual test',
  resolver: 'A path to a custom resolver',
  restoreMocks: 'Automatically restore mock state and implementation before every test',
  rootDir: 'The root directory that Jest should scan for tests and modules within',
  roots: 'A list of paths to directories that Jest should use to search for files in',
  runner: "Allows you to use a custom runner instead of Jest's default test runner",
  setupFiles: 'The paths to modules that run some code to configure or set up the testing environment before each test',
  setupFilesAfterEnv: 'A list of paths to modules that run some code to configure or set up the testing framework before each test',
  slowTestThreshold: 'The number of seconds after which a test is considered as slow and reported as such in the results.',
  snapshotSerializers: 'A list of paths to snapshot serializer modules Jest should use for snapshot testing',
  testEnvironment: 'The test environment that will be used for testing',
  testEnvironmentOptions: 'Options that will be passed to the testEnvironment',
  testLocationInResults: 'Adds a location field to test results',
  testMatch: 'The glob patterns Jest uses to detect test files',
  testPathIgnorePatterns: 'An array of regexp pattern strings that are matched against all test paths, matched tests are skipped',
  testRegex: 'The regexp pattern or array of patterns that Jest uses to detect test files',
  testResultsProcessor: 'This option allows the use of a custom results processor',
  testRunner: 'This option allows use of a custom test runner',
  transform: 'A map from regular expressions to paths to transformers',
  transformIgnorePatterns: 'An array of regexp pattern strings that are matched against all source file paths, matched files will skip transformation',
  unmockedModulePathPatterns: 'An array of regexp pattern strings that are matched against all modules before the module loader will automatically return a mock for them',
  verbose: 'Indicates whether each individual test should be reported during the run',
  watchPathIgnorePatterns: 'An array of regexp patterns that are matched against all source file paths before re-running tests in watch mode',
  watchman: 'Whether to use watchman for file crawling'
};
var _default = exports["default"] = descriptions;

/***/ },

/***/ "./src/ReporterValidationErrors.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.createArrayReporterError = createArrayReporterError;
exports.createReporterError = createReporterError;
exports.validateReporters = validateReporters;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
function _jestValidate() {
  const data = require("jest-validate");
  _jestValidate = function () {
    return data;
  };
  return data;
}
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const validReporterTypes = ['array', 'string'];
const ERROR = `${_utils.BULLET}Reporter Validation Error`;

/**
 * Reporter Validation Error is thrown if the given arguments
 * within the reporter are not valid.
 *
 * This is a highly specific reporter error and in the future will be
 * merged with jest-validate. Till then, we can make use of it. It works
 * and that's what counts most at this time.
 */
function createReporterError(reporterIndex, reporterValue) {
  const errorMessage = `  Reporter at index ${reporterIndex} must be of type:\n` + `    ${_chalk().default.bold.green(validReporterTypes.join(' or '))}\n` + '  but instead received:\n' + `    ${_chalk().default.bold.red((0, _getType().getType)(reporterValue))}`;
  return new (_jestValidate().ValidationError)(ERROR, errorMessage, _utils.DOCUMENTATION_NOTE);
}
function createArrayReporterError(arrayReporter, reporterIndex, valueIndex, value, expectedType, valueName) {
  const errorMessage = `  Unexpected value for ${valueName} ` + `at index ${valueIndex} of reporter at index ${reporterIndex}\n` + '  Expected:\n' + `    ${_chalk().default.bold.red(expectedType)}\n` + '  Got:\n' + `    ${_chalk().default.bold.green((0, _getType().getType)(value))}\n` + '  Reporter configuration:\n' + `    ${_chalk().default.bold.green(JSON.stringify(arrayReporter, null, 2).split('\n').join('\n    '))}`;
  return new (_jestValidate().ValidationError)(ERROR, errorMessage, _utils.DOCUMENTATION_NOTE);
}
function validateReporters(reporterConfig) {
  return reporterConfig.every((reporter, index) => {
    if (Array.isArray(reporter)) {
      validateArrayReporter(reporter, index);
    } else if (typeof reporter !== 'string') {
      throw createReporterError(index, reporter);
    }
    return true;
  });
}
function validateArrayReporter(arrayReporter, reporterIndex) {
  const [path, options] = arrayReporter;
  if (typeof path !== 'string') {
    throw createArrayReporterError(arrayReporter, reporterIndex, 0, path, 'string', 'Path');
  } else if (typeof options !== 'object') {
    throw createArrayReporterError(arrayReporter, reporterIndex, 1, options, 'object', 'Reporter Configuration');
  }
}

/***/ },

/***/ "./src/ValidConfig.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.initialProjectOptions = exports.initialOptions = void 0;
function _jestRegexUtil() {
  const data = require("jest-regex-util");
  _jestRegexUtil = function () {
    return data;
  };
  return data;
}
function _jestValidate() {
  const data = require("jest-validate");
  _jestValidate = function () {
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
var _constants = __webpack_require__("./src/constants.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const NODE_MODULES_REGEXP = (0, _jestRegexUtil().replacePathSepForRegex)(_constants.NODE_MODULES);
const initialOptions = exports.initialOptions = {
  automock: false,
  bail: (0, _jestValidate().multipleValidOptions)(false, 0),
  cache: true,
  cacheDirectory: '/tmp/user/jest',
  changedFilesWithAncestor: false,
  changedSince: 'master',
  ci: false,
  clearMocks: false,
  collectCoverage: true,
  collectCoverageFrom: ['src', '!public'],
  collectTests: false,
  coverageDirectory: 'coverage',
  coveragePathIgnorePatterns: [NODE_MODULES_REGEXP],
  coverageProvider: 'v8',
  coverageReporters: ['json', 'text', 'lcov', 'clover'],
  coverageThreshold: {
    global: {
      branches: 50,
      functions: 100,
      lines: 100,
      statements: 100
    }
  },
  dependencyExtractor: '<rootDir>/dependencyExtractor.js',
  detectLeaks: false,
  detectOpenHandles: false,
  displayName: (0, _jestValidate().multipleValidOptions)('test-config', {
    color: 'blue',
    name: 'test-config'
  }),
  errorOnDeprecated: false,
  expand: false,
  extensionsToTreatAsEsm: [],
  fakeTimers: {
    advanceTimers: (0, _jestValidate().multipleValidOptions)(40, true),
    doNotFake: ['Date', 'hrtime', 'nextTick', 'performance', 'queueMicrotask', 'requestAnimationFrame', 'cancelAnimationFrame', 'requestIdleCallback', 'cancelIdleCallback', 'setImmediate', 'clearImmediate', 'setInterval', 'clearInterval', 'setTimeout', 'clearTimeout'],
    enableGlobally: true,
    legacyFakeTimers: false,
    now: 1_483_228_800_000,
    timerLimit: 1000
  },
  filter: '<rootDir>/filter.js',
  forceCoverageMatch: ['**/*.t.js'],
  forceExit: false,
  globalSetup: 'setup.js',
  globalTeardown: 'teardown.js',
  globals: {
    __DEV__: true
  },
  haste: {
    computeSha1: true,
    defaultPlatform: 'ios',
    enableSymlinks: false,
    forceNodeFilesystemAPI: true,
    hasteImplModulePath: '<rootDir>/haste_impl.js',
    hasteMapModulePath: '',
    platforms: ['ios', 'android'],
    retainAllFiles: false,
    throwOnModuleCollision: false
  },
  id: 'string',
  injectGlobals: true,
  json: false,
  lastCommit: false,
  listTests: false,
  logHeapUsage: true,
  maxConcurrency: 5,
  maxWorkers: '50%',
  moduleDirectories: ['node_modules'],
  moduleFileExtensions: ['js', 'mjs', 'cjs', 'json', 'jsx', 'ts', 'mts', 'cts', 'tsx', 'node'],
  moduleNameMapper: {
    '^React$': '<rootDir>/node_modules/react'
  },
  modulePathIgnorePatterns: ['<rootDir>/build/'],
  modulePaths: ['/shared/vendor/modules'],
  noStackTrace: false,
  notify: false,
  notifyMode: 'failure-change',
  onlyChanged: false,
  onlyFailures: false,
  openHandlesTimeout: 1000,
  passWithNoTests: false,
  preset: 'react-native',
  prettierPath: '<rootDir>/node_modules/prettier',
  projects: ['project-a', 'project-b/'],
  randomize: false,
  reporters: ['default', 'custom-reporter-1', ['custom-reporter-2', {
    configValue: true
  }]],
  resetMocks: false,
  resetModules: false,
  resolver: '<rootDir>/resolver.js',
  restoreMocks: false,
  rootDir: '/',
  roots: ['<rootDir>'],
  runTestsByPath: false,
  runner: (0, _jestValidate().multipleValidOptions)('jest-runner', ['jest-runner', ['jest-runner', {
    option: true
  }]]),
  runtime: '<rootDir>',
  sandboxInjectedGlobals: [],
  setupFiles: ['<rootDir>/setup.js'],
  setupFilesAfterEnv: ['<rootDir>/testSetupFile.js'],
  showSeed: false,
  silent: true,
  skipFilter: false,
  skipNodeResolution: false,
  slowTestThreshold: 5,
  snapshotFormat: _prettyFormat().DEFAULT_OPTIONS,
  snapshotResolver: '<rootDir>/snapshotResolver.js',
  snapshotSerializers: ['my-serializer-module'],
  testEnvironment: 'jest-environment-node',
  testEnvironmentOptions: {
    url: 'http://localhost',
    userAgent: 'Agent/007'
  },
  testFailureExitCode: 1,
  testLocationInResults: false,
  testMatch: (0, _jestValidate().multipleValidOptions)('**/__tests__/**/?(*.)+(spec|test).?([mc])[jt]s?(x)', ['**/__tests__/**/*.?([mc])[jt]s?(x)', '**/?(*.)+(spec|test).?([mc])[jt]s?(x)']),
  testNamePattern: 'test signature',
  testPathIgnorePatterns: [NODE_MODULES_REGEXP],
  testRegex: (0, _jestValidate().multipleValidOptions)('(/__tests__/.*|(\\.|/)(test|spec))\\.[mc]?[jt]sx?$', ['/__tests__/\\.test\\.[mc]?[jt]sx?$', '/__tests__/\\.spec\\.[mc]?[jt]sx?$']),
  testResultsProcessor: 'processor-node-module',
  testRunner: 'circus',
  testSequencer: '@jest/test-sequencer',
  testTimeout: 5000,
  transform: {
    '\\.js$': '<rootDir>/preprocessor.js'
  },
  transformIgnorePatterns: [NODE_MODULES_REGEXP],
  unmockedModulePathPatterns: ['mock'],
  updateSnapshot: true,
  useStderr: false,
  verbose: false,
  waitForUnhandledRejections: false,
  watch: false,
  watchAll: false,
  watchPathIgnorePatterns: ['<rootDir>/e2e/'],
  watchPlugins: ['path/to/yourWatchPlugin', ['jest-watch-typeahead/filename', {
    key: 'k',
    prompt: 'do something with my custom prompt'
  }]],
  watchman: true,
  workerGracefulExitTimeout: 500,
  workerIdleMemoryLimit: (0, _jestValidate().multipleValidOptions)(0.2, '50%'),
  workerThreads: true
};
const initialProjectOptions = exports.initialProjectOptions = {
  automock: false,
  cache: true,
  cacheDirectory: '/tmp/user/jest',
  clearMocks: false,
  collectCoverage: false,
  collectCoverageFrom: ['src', '!public'],
  coverageDirectory: 'coverage',
  coveragePathIgnorePatterns: [NODE_MODULES_REGEXP],
  coverageProvider: 'v8',
  coverageReporters: ['json', 'text', 'lcov', 'clover'],
  dependencyExtractor: '<rootDir>/dependencyExtractor.js',
  detectLeaks: false,
  detectOpenHandles: false,
  displayName: (0, _jestValidate().multipleValidOptions)('test-config', {
    color: 'blue',
    name: 'test-config'
  }),
  errorOnDeprecated: false,
  extensionsToTreatAsEsm: [],
  fakeTimers: {
    advanceTimers: (0, _jestValidate().multipleValidOptions)(40, true),
    doNotFake: ['Date', 'hrtime', 'nextTick', 'performance', 'queueMicrotask', 'requestAnimationFrame', 'cancelAnimationFrame', 'requestIdleCallback', 'cancelIdleCallback', 'setImmediate', 'clearImmediate', 'setInterval', 'clearInterval', 'setTimeout', 'clearTimeout'],
    enableGlobally: true,
    legacyFakeTimers: false,
    now: 1_483_228_800_000,
    timerLimit: 1000
  },
  filter: '<rootDir>/filter.js',
  forceCoverageMatch: ['**/*.t.js'],
  globalSetup: 'setup.js',
  globalTeardown: 'teardown.js',
  globals: {
    __DEV__: true
  },
  haste: {
    computeSha1: true,
    defaultPlatform: 'ios',
    enableSymlinks: false,
    forceNodeFilesystemAPI: true,
    hasteImplModulePath: '<rootDir>/haste_impl.js',
    hasteMapModulePath: '',
    platforms: ['ios', 'android'],
    retainAllFiles: false,
    throwOnModuleCollision: false
  },
  id: 'string',
  injectGlobals: true,
  moduleDirectories: ['node_modules'],
  moduleFileExtensions: ['js', 'mjs', 'cjs', 'json', 'jsx', 'ts', 'mts', 'cts', 'tsx', 'node'],
  moduleNameMapper: {
    '^React$': '<rootDir>/node_modules/react'
  },
  modulePathIgnorePatterns: ['<rootDir>/build/'],
  modulePaths: ['/shared/vendor/modules'],
  openHandlesTimeout: 1000,
  preset: 'react-native',
  prettierPath: '<rootDir>/node_modules/prettier',
  reporters: ['default', 'custom-reporter-1', ['custom-reporter-2', {
    configValue: true
  }]],
  resetMocks: false,
  resetModules: false,
  resolver: '<rootDir>/resolver.js',
  restoreMocks: false,
  rootDir: '/',
  roots: ['<rootDir>'],
  runner: (0, _jestValidate().multipleValidOptions)('jest-runner', ['jest-runner', ['jest-runner', {
    option: true
  }]]),
  runtime: '<rootDir>',
  sandboxInjectedGlobals: [],
  setupFiles: ['<rootDir>/setup.js'],
  setupFilesAfterEnv: ['<rootDir>/testSetupFile.js'],
  silent: false,
  skipFilter: false,
  skipNodeResolution: false,
  slowTestThreshold: 5,
  snapshotFormat: _prettyFormat().DEFAULT_OPTIONS,
  snapshotResolver: '<rootDir>/snapshotResolver.js',
  snapshotSerializers: ['my-serializer-module'],
  testEnvironment: 'jest-environment-node',
  testEnvironmentOptions: {
    url: 'http://localhost',
    userAgent: 'Agent/007'
  },
  testLocationInResults: false,
  testMatch: ['**/__tests__/**/*.?([mc])[jt]s?(x)', '**/?(*.)+(spec|test).?([mc])[jt]s?(x)'],
  testPathIgnorePatterns: [NODE_MODULES_REGEXP],
  testRegex: (0, _jestValidate().multipleValidOptions)('(/__tests__/.*|(\\.|/)(test|spec))\\.[mc]?[jt]sx?$', ['/__tests__/\\.test\\.[mc]?[jt]sx?$', '/__tests__/\\.spec\\.[mc]?[jt]sx?$']),
  testRunner: 'circus',
  testTimeout: 5000,
  transform: {
    '\\.js$': '<rootDir>/preprocessor.js'
  },
  transformIgnorePatterns: [NODE_MODULES_REGEXP],
  unmockedModulePathPatterns: ['mock'],
  verbose: false,
  waitForUnhandledRejections: false,
  watchPathIgnorePatterns: ['<rootDir>/e2e/'],
  workerIdleMemoryLimit: (0, _jestValidate().multipleValidOptions)(0.2, '50%')
};

/***/ },

/***/ "./src/color.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.getDisplayNameColor = void 0;
function _nodeCrypto() {
  const data = require("node:crypto");
  _nodeCrypto = function () {
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

const colors = ['red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white'];
const getDisplayNameColor = seed => {
  if (seed === undefined) {
    return 'white';
  }
  const hash = (0, _nodeCrypto().createHash)('sha256');
  hash.update(seed);
  const num = hash.digest().readUInt32LE(0);
  return colors[num % colors.length];
};
exports.getDisplayNameColor = getDisplayNameColor;

/***/ },

/***/ "./src/constants.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.PACKAGE_JSON = exports.NODE_MODULES = exports.JEST_CONFIG_EXT_TS = exports.JEST_CONFIG_EXT_ORDER = exports.JEST_CONFIG_EXT_MTS = exports.JEST_CONFIG_EXT_MJS = exports.JEST_CONFIG_EXT_JSON = exports.JEST_CONFIG_EXT_JS = exports.JEST_CONFIG_EXT_CTS = exports.JEST_CONFIG_EXT_CJS = exports.JEST_CONFIG_BASE_NAME = exports.DEFAULT_JS_PATTERN = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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

const NODE_MODULES = exports.NODE_MODULES = `${path().sep}node_modules${path().sep}`;
const DEFAULT_JS_PATTERN = exports.DEFAULT_JS_PATTERN = '\\.[jt]sx?$';
const PACKAGE_JSON = exports.PACKAGE_JSON = 'package.json';
const JEST_CONFIG_BASE_NAME = exports.JEST_CONFIG_BASE_NAME = 'jest.config';
const JEST_CONFIG_EXT_CJS = exports.JEST_CONFIG_EXT_CJS = '.cjs';
const JEST_CONFIG_EXT_MJS = exports.JEST_CONFIG_EXT_MJS = '.mjs';
const JEST_CONFIG_EXT_MTS = exports.JEST_CONFIG_EXT_MTS = '.mts';
const JEST_CONFIG_EXT_JS = exports.JEST_CONFIG_EXT_JS = '.js';
const JEST_CONFIG_EXT_TS = exports.JEST_CONFIG_EXT_TS = '.ts';
const JEST_CONFIG_EXT_CTS = exports.JEST_CONFIG_EXT_CTS = '.cts';
const JEST_CONFIG_EXT_JSON = exports.JEST_CONFIG_EXT_JSON = '.json';
const JEST_CONFIG_EXT_ORDER = exports.JEST_CONFIG_EXT_ORDER = Object.freeze([JEST_CONFIG_EXT_JS, JEST_CONFIG_EXT_TS, JEST_CONFIG_EXT_MJS, JEST_CONFIG_EXT_MTS, JEST_CONFIG_EXT_CJS, JEST_CONFIG_EXT_CTS, JEST_CONFIG_EXT_JSON]);

/***/ },

/***/ "./src/getCacheDirectory.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
    return data;
  };
  return data;
}
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const getCacheDirectory = () => {
  const {
    getuid
  } = process;
  const tmpdirPath = path().join((0, _jestUtil().tryRealpath)((0, _nodeOs().tmpdir)()), 'jest');
  if (getuid == null) {
    return tmpdirPath;
  } else {
    // On some platforms tmpdir() is `/tmp`, causing conflicts between different
    // users and permission issues. Adding an additional subdivision by UID can
    // help.
    return `${tmpdirPath}_${getuid.call(process).toString(36)}`;
  }
};
var _default = exports["default"] = getCacheDirectory;

/***/ },

/***/ "./src/getMaxWorkers.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getMaxWorkers;
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
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

function getMaxWorkers(argv, defaultOptions) {
  if (argv.runInBand) {
    return 1;
  } else if (argv.maxWorkers) {
    return parseWorkers(argv.maxWorkers);
  } else if (defaultOptions && defaultOptions.maxWorkers) {
    return parseWorkers(defaultOptions.maxWorkers);
  } else {
    // In watch mode, Jest should be unobtrusive and not use all available CPUs.
    const numCpus = (0, _nodeOs().availableParallelism)();
    const isWatchModeEnabled = argv.watch || argv.watchAll;
    return Math.max(isWatchModeEnabled ? Math.floor(numCpus / 2) : numCpus - 1, 1);
  }
}
const parseWorkers = maxWorkers => {
  const parsed = Number.parseInt(maxWorkers.toString(), 10);
  if (typeof maxWorkers === 'string' && maxWorkers.trim().endsWith('%') && parsed > 0 && parsed <= 100) {
    const numCpus = (0, _nodeOs().availableParallelism)();
    const workers = Math.floor(parsed / 100 * numCpus);
    return Math.max(workers, 1);
  }
  return parsed > 0 ? parsed : 1;
};

/***/ },

/***/ "./src/index.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.constants = void 0;
Object.defineProperty(exports, "defaults", ({
  enumerable: true,
  get: function () {
    return _Defaults.default;
  }
}));
exports.defineConfig = defineConfig;
Object.defineProperty(exports, "deprecationEntries", ({
  enumerable: true,
  get: function () {
    return _Deprecated.default;
  }
}));
Object.defineProperty(exports, "descriptions", ({
  enumerable: true,
  get: function () {
    return _Descriptions.default;
  }
}));
Object.defineProperty(exports, "isJSONString", ({
  enumerable: true,
  get: function () {
    return _utils.isJSONString;
  }
}));
exports.mergeConfig = mergeConfig;
Object.defineProperty(exports, "normalize", ({
  enumerable: true,
  get: function () {
    return _normalize.default;
  }
}));
exports.readConfig = readConfig;
exports.readConfigs = readConfigs;
exports.readInitialOptions = readInitialOptions;
Object.defineProperty(exports, "replaceRootDirInPath", ({
  enumerable: true,
  get: function () {
    return _utils.replaceRootDirInPath;
  }
}));
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
function _deepmerge() {
  const data = _interopRequireDefault(require("deepmerge"));
  _deepmerge = function () {
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
var constants = _interopRequireWildcard(__webpack_require__("./src/constants.ts"));
exports.constants = constants;
var _normalize = _interopRequireDefault(__webpack_require__("./src/normalize.ts"));
var _readConfigFileAndSetRootDir = _interopRequireDefault(__webpack_require__("./src/readConfigFileAndSetRootDir.ts"));
var _resolveConfigPath = _interopRequireDefault(__webpack_require__("./src/resolveConfigPath.ts"));
var _utils = __webpack_require__("./src/utils.ts");
var _Deprecated = _interopRequireDefault(__webpack_require__("./src/Deprecated.ts"));
var _Defaults = _interopRequireDefault(__webpack_require__("./src/Defaults.ts"));
var _Descriptions = _interopRequireDefault(__webpack_require__("./src/Descriptions.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/**
 * Type helper to make it easier to use Jest config accepts a direct JestTestConfigObject object, or a function that returns it. The function receives a JestTestConfigObject object.
 */

function defineConfig(config) {
  return config;
}

/**
 * Merges two configuration objects, where the second object takes precedence over the first one.
 */
function mergeConfig(defaults, overrides) {
  if (typeof defaults === 'function' || typeof overrides === 'function') {
    throw new TypeError('Cannot merge config in form of callback');
  }
  return _deepmerge().default.all([defaults, overrides]);
}
async function readConfig(argv, packageRootOrConfig,
// Whether it needs to look into `--config` arg passed to CLI.
// It only used to read initial config. If the initial config contains
// `project` property, we don't want to read `--config` value and rather
// read individual configs for every project.
skipArgvConfigOption, parentConfigDirname, projectIndex = Number.POSITIVE_INFINITY, skipMultipleConfigError = false) {
  const {
    config: initialOptions,
    configPath
  } = await readInitialOptions(argv.config, {
    packageRootOrConfig,
    parentConfigDirname,
    readFromCwd: skipArgvConfigOption,
    skipMultipleConfigError
  });
  const packageRoot = typeof packageRootOrConfig === 'string' ? path().resolve(packageRootOrConfig) : undefined;
  const {
    options,
    hasDeprecationWarnings
  } = await (0, _normalize.default)(initialOptions, argv, configPath, projectIndex, skipArgvConfigOption && !(packageRoot === parentConfigDirname));
  const {
    globalConfig,
    projectConfig
  } = groupOptions(options);
  return {
    configPath,
    globalConfig,
    hasDeprecationWarnings,
    projectConfig
  };
}
const groupOptions = options => ({
  globalConfig: Object.freeze({
    bail: options.bail,
    changedFilesWithAncestor: options.changedFilesWithAncestor,
    changedSince: options.changedSince,
    ci: options.ci,
    collectCoverage: options.collectCoverage,
    collectCoverageFrom: options.collectCoverageFrom,
    collectTests: options.collectTests,
    coverageDirectory: options.coverageDirectory,
    coverageProvider: options.coverageProvider,
    coverageReporters: options.coverageReporters,
    coverageThreshold: options.coverageThreshold,
    detectLeaks: options.detectLeaks,
    detectOpenHandles: options.detectOpenHandles,
    errorOnDeprecated: options.errorOnDeprecated,
    expand: options.expand,
    filter: options.filter,
    findRelatedTests: options.findRelatedTests,
    forceExit: options.forceExit,
    globalSetup: options.globalSetup,
    globalTeardown: options.globalTeardown,
    json: options.json,
    lastCommit: options.lastCommit,
    listTests: options.listTests,
    logHeapUsage: options.logHeapUsage,
    maxConcurrency: options.maxConcurrency,
    maxWorkers: options.maxWorkers,
    noSCM: undefined,
    noStackTrace: options.noStackTrace,
    nonFlagArgs: options.nonFlagArgs,
    notify: options.notify,
    notifyMode: options.notifyMode,
    onlyChanged: options.onlyChanged,
    onlyFailures: options.onlyFailures,
    openHandlesTimeout: options.openHandlesTimeout,
    outputFile: options.outputFile,
    passWithNoTests: options.passWithNoTests,
    projects: options.projects,
    randomize: options.randomize,
    replname: options.replname,
    reporters: options.reporters,
    rootDir: options.rootDir,
    runInBand: options.runInBand,
    runTestsByPath: options.runTestsByPath,
    seed: options.seed,
    shard: options.shard,
    showSeed: options.showSeed,
    silent: options.silent,
    skipFilter: options.skipFilter,
    snapshotFormat: options.snapshotFormat,
    testFailureExitCode: options.testFailureExitCode,
    testNamePattern: options.testNamePattern,
    testPathPatterns: options.testPathPatterns,
    testResultsProcessor: options.testResultsProcessor,
    testSequencer: options.testSequencer,
    testTimeout: options.testTimeout,
    updateSnapshot: options.updateSnapshot,
    useStderr: options.useStderr,
    verbose: options.verbose,
    waitForUnhandledRejections: options.waitForUnhandledRejections,
    watch: options.watch,
    watchAll: options.watchAll,
    watchPlugins: options.watchPlugins,
    watchman: options.watchman,
    workerGracefulExitTimeout: options.workerGracefulExitTimeout,
    workerIdleMemoryLimit: options.workerIdleMemoryLimit,
    workerThreads: options.workerThreads
  }),
  projectConfig: Object.freeze({
    automock: options.automock,
    cache: options.cache,
    cacheDirectory: options.cacheDirectory,
    clearMocks: options.clearMocks,
    collectCoverage: options.collectCoverage,
    collectCoverageFrom: options.collectCoverageFrom,
    coverageDirectory: options.coverageDirectory,
    coveragePathIgnorePatterns: options.coveragePathIgnorePatterns,
    coverageProvider: options.coverageProvider,
    coverageReporters: options.coverageReporters,
    cwd: options.cwd,
    dependencyExtractor: options.dependencyExtractor,
    detectLeaks: options.detectLeaks,
    detectOpenHandles: options.detectOpenHandles,
    displayName: options.displayName,
    errorOnDeprecated: options.errorOnDeprecated,
    extensionsToTreatAsEsm: options.extensionsToTreatAsEsm,
    fakeTimers: options.fakeTimers,
    filter: options.filter,
    forceCoverageMatch: options.forceCoverageMatch,
    globalSetup: options.globalSetup,
    globalTeardown: options.globalTeardown,
    globals: options.globals,
    haste: options.haste,
    id: options.id,
    injectGlobals: options.injectGlobals,
    moduleDirectories: options.moduleDirectories,
    moduleFileExtensions: options.moduleFileExtensions,
    moduleNameMapper: options.moduleNameMapper,
    modulePathIgnorePatterns: options.modulePathIgnorePatterns,
    modulePaths: options.modulePaths,
    openHandlesTimeout: options.openHandlesTimeout,
    prettierPath: options.prettierPath,
    reporters: options.reporters,
    resetMocks: options.resetMocks,
    resetModules: options.resetModules,
    resolver: options.resolver,
    restoreMocks: options.restoreMocks,
    rootDir: options.rootDir,
    roots: options.roots,
    runner: options.runner,
    runnerOptions: options.runnerOptions,
    runtime: options.runtime,
    sandboxInjectedGlobals: options.sandboxInjectedGlobals,
    setupFiles: options.setupFiles,
    setupFilesAfterEnv: options.setupFilesAfterEnv,
    silent: options.silent,
    skipFilter: options.skipFilter,
    skipNodeResolution: options.skipNodeResolution,
    slowTestThreshold: options.slowTestThreshold,
    snapshotFormat: options.snapshotFormat,
    snapshotResolver: options.snapshotResolver,
    snapshotSerializers: options.snapshotSerializers,
    testEnvironment: options.testEnvironment,
    testEnvironmentOptions: options.testEnvironmentOptions,
    testLocationInResults: options.testLocationInResults,
    testMatch: options.testMatch,
    testPathIgnorePatterns: options.testPathIgnorePatterns,
    testRegex: options.testRegex,
    testRunner: options.testRunner,
    testTimeout: options.testTimeout,
    transform: options.transform,
    transformIgnorePatterns: options.transformIgnorePatterns,
    unmockedModulePathPatterns: options.unmockedModulePathPatterns,
    verbose: options.verbose,
    waitForUnhandledRejections: options.waitForUnhandledRejections,
    watchPathIgnorePatterns: options.watchPathIgnorePatterns
  })
});
const ensureNoDuplicateConfigs = (parsedConfigs, projects) => {
  if (projects.length <= 1) {
    return;
  }
  const configPathMap = new Map();
  for (const config of parsedConfigs) {
    const {
      configPath
    } = config;
    if (configPathMap.has(configPath)) {
      const message = `Whoops! Two projects resolved to the same config path: ${_chalk().default.bold(String(configPath))}:

  Project 1: ${_chalk().default.bold(projects[parsedConfigs.indexOf(config)])}
  Project 2: ${_chalk().default.bold(projects[parsedConfigs.indexOf(configPathMap.get(configPath))])}

This usually means that your ${_chalk().default.bold('"projects"')} config includes a directory that doesn't have any configuration recognizable by Jest. Please fix it.
`;
      throw new Error(message);
    }
    if (configPath !== null) {
      configPathMap.set(configPath, config);
    }
  }
};
/**
 * Reads the jest config, without validating them or filling it out with defaults.
 * @param config The path to the file or serialized config.
 * @param param1 Additional options
 * @returns The raw initial config (not validated)
 */
async function readInitialOptions(config, {
  packageRootOrConfig = process.cwd(),
  parentConfigDirname = null,
  readFromCwd = false,
  skipMultipleConfigError = false
} = {}) {
  if (typeof packageRootOrConfig !== 'string') {
    if (parentConfigDirname) {
      const rawOptions = packageRootOrConfig;
      rawOptions.rootDir = rawOptions.rootDir ? (0, _utils.replaceRootDirInPath)(parentConfigDirname, rawOptions.rootDir) : parentConfigDirname;
      return {
        config: rawOptions,
        configPath: null
      };
    } else {
      throw new Error('Jest: Cannot use configuration as an object without a file path.');
    }
  }
  if ((0, _utils.isJSONString)(config)) {
    try {
      // A JSON string was passed to `--config` argument and we can parse it
      // and use as is.
      const initialOptions = JSON.parse(config);
      // NOTE: we might need to resolve this dir to an absolute path in the future
      initialOptions.rootDir = initialOptions.rootDir || packageRootOrConfig;
      return {
        config: initialOptions,
        configPath: null
      };
    } catch {
      throw new Error('There was an error while parsing the `--config` argument as a JSON string.');
    }
  }
  if (!readFromCwd && typeof config == 'string') {
    // A string passed to `--config`, which is either a direct path to the config
    // or a path to directory containing `package.json`, `jest.config.js` or `jest.config.ts`
    const configPath = (0, _resolveConfigPath.default)(config, process.cwd(), skipMultipleConfigError);
    return {
      config: await (0, _readConfigFileAndSetRootDir.default)(configPath),
      configPath
    };
  }
  // Otherwise just try to find config in the current rootDir.
  const configPath = (0, _resolveConfigPath.default)(packageRootOrConfig, process.cwd(), skipMultipleConfigError);
  return {
    config: await (0, _readConfigFileAndSetRootDir.default)(configPath),
    configPath
  };
}

// Possible scenarios:
//  1. jest --config config.json
//  2. jest --projects p1 p2
//  3. jest --projects p1 p2 --config config.json
//  4. jest --projects p1
//  5. jest
//
// If no projects are specified, process.cwd() will be used as the default
// (and only) project.
async function readConfigs(argv, projectPaths) {
  let globalConfig;
  let hasDeprecationWarnings;
  let configs = [];
  let projects = projectPaths;
  let configPath;
  if (projectPaths.length === 1) {
    const parsedConfig = await readConfig(argv, projects[0]);
    configPath = parsedConfig.configPath;
    hasDeprecationWarnings = parsedConfig.hasDeprecationWarnings;
    globalConfig = parsedConfig.globalConfig;
    configs = [parsedConfig.projectConfig];
    if (globalConfig.projects && globalConfig.projects.length > 0) {
      // Even though we had one project in CLI args, there might be more
      // projects defined in the config.
      // In other words, if this was a single project,
      // and its config has `projects` settings, use that value instead.
      projects = globalConfig.projects;
    }
  }
  if (projects.length > 0) {
    const cwd = process.platform === 'win32' ? (0, _jestUtil().tryRealpath)(process.cwd()) : process.cwd();
    const projectIsCwd = projects[0] === cwd;
    const parsedConfigs = await Promise.all(projects.filter(root => {
      // Ignore globbed files that cannot be `require`d.
      if (typeof root === 'string' && fs().existsSync(root) && !fs().lstatSync(root).isDirectory() && !constants.JEST_CONFIG_EXT_ORDER.some(ext => root.endsWith(ext))) {
        return false;
      }
      return true;
    }).map((root, projectIndex) => {
      const projectIsTheOnlyProject = projectIndex === 0 && projects.length === 1;
      const skipArgvConfigOption = !(projectIsTheOnlyProject && projectIsCwd);
      return readConfig(argv, root, skipArgvConfigOption, configPath ? path().dirname(configPath) : cwd, projectIndex,
      // we wanna skip the warning if this is the "main" project
      projectIsCwd);
    }));
    ensureNoDuplicateConfigs(parsedConfigs, projects);
    configs = parsedConfigs.map(({
      projectConfig
    }) => projectConfig);
    if (!hasDeprecationWarnings) {
      hasDeprecationWarnings = parsedConfigs.some(({
        hasDeprecationWarnings
      }) => !!hasDeprecationWarnings);
    }
    // If no config was passed initially, use the one from the first project
    if (!globalConfig) {
      globalConfig = parsedConfigs[0].globalConfig;
    }
  }
  if (!globalConfig || configs.length === 0) {
    throw new Error('jest: No configuration found for any project.');
  }
  return {
    configs,
    globalConfig,
    hasDeprecationWarnings: !!hasDeprecationWarnings
  };
}

/***/ },

/***/ "./src/normalize.ts"
(module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = normalize;
function _nodeCrypto() {
  const data = require("node:crypto");
  _nodeCrypto = function () {
    return data;
  };
  return data;
}
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
    return data;
  };
  return data;
}
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
function _deepmerge() {
  const data = _interopRequireDefault(require("deepmerge"));
  _deepmerge = function () {
    return data;
  };
  return data;
}
function _glob() {
  const data = require("glob");
  _glob = function () {
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
function _pattern() {
  const data = require("@jest/pattern");
  _pattern = function () {
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
function _jestResolve() {
  const data = _interopRequireWildcard(require("jest-resolve"));
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
function _jestValidate() {
  const data = require("jest-validate");
  _jestValidate = function () {
    return data;
  };
  return data;
}
var _Defaults = _interopRequireDefault(__webpack_require__("./src/Defaults.ts"));
var _Deprecated = _interopRequireDefault(__webpack_require__("./src/Deprecated.ts"));
var _ReporterValidationErrors = __webpack_require__("./src/ReporterValidationErrors.ts");
var _ValidConfig = __webpack_require__("./src/ValidConfig.ts");
var _color = __webpack_require__("./src/color.ts");
var _constants = __webpack_require__("./src/constants.ts");
var _getMaxWorkers = _interopRequireDefault(__webpack_require__("./src/getMaxWorkers.ts"));
var _parseShardPair = __webpack_require__("./src/parseShardPair.ts");
var _setFromArgv = _interopRequireDefault(__webpack_require__("./src/setFromArgv.ts"));
var _stringToBytes = _interopRequireDefault(__webpack_require__("./src/stringToBytes.ts"));
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const ERROR = `${_utils.BULLET}Validation Error`;
const PRESET_EXTENSIONS = ['.json', '.js', '.cjs', '.mjs'];
const PRESET_NAME = 'jest-preset';
const GLOBAL_ONLY_OPTIONS = new Set(Object.keys(_ValidConfig.initialOptions).filter(key => !Object.hasOwn(_ValidConfig.initialProjectOptions, key)));
const unknownProjectOption = (config, exampleConfig, option, options, path) => {
  const warningTitle = (options.title && options.title.warning) ?? 'Validation Warning';
  const optionPath = path && path.length > 0 ? `${path.join('.')}.` : '';
  if ((!path || path.length === 0) && GLOBAL_ONLY_OPTIONS.has(option)) {
    (0, _jestValidate().logValidationWarning)(warningTitle, `  Option ${_chalk().default.bold(`"${optionPath}${option}"`)} is not supported in an individual project configuration.\n  Move it to the root configuration.`, options.comment);
  } else {
    const didYouMean = (0, _jestValidate().createDidYouMeanMessage)(option, Object.keys(exampleConfig));
    (0, _jestValidate().logValidationWarning)(warningTitle, `  Unknown option ${_chalk().default.bold(`"${optionPath}${option}"`)} with value ${_chalk().default.bold((0, _jestValidate().format)(config[option]))} was found.${didYouMean && ` ${didYouMean}`}\n  This is probably a typing mistake. Fixing it will remove this message.`, options.comment);
  }
};
const createConfigError = message => new (_jestValidate().ValidationError)(ERROR, message, _utils.DOCUMENTATION_NOTE);

// we wanna avoid webpack trying to be clever
const requireResolve = module => require.resolve(module);
function verifyDirectoryExists(path, key) {
  try {
    const rootStat = (0, _gracefulFs().statSync)(path);
    if (!rootStat.isDirectory()) {
      throw createConfigError(`  ${_chalk().default.bold(path)} in the ${_chalk().default.bold(key)} option is not a directory.`);
    }
  } catch (error) {
    if (error instanceof _jestValidate().ValidationError) {
      throw error;
    }
    if (error.code === 'ENOENT') {
      throw createConfigError(`  Directory ${_chalk().default.bold(path)} in the ${_chalk().default.bold(key)} option was not found.`);
    }

    // Not sure in which cases `statSync` can throw, so let's just show the underlying error to the user
    throw createConfigError(`  Got an error trying to find ${_chalk().default.bold(path)} in the ${_chalk().default.bold(key)} option.\n\n  Error was: ${error.message}`);
  }
}
const mergeOptionWithPreset = (options, preset, optionName) => {
  if (options[optionName] && preset[optionName]) {
    options[optionName] = {
      ...options[optionName],
      ...preset[optionName],
      ...options[optionName]
    };
  }
};
const mergeGlobalsWithPreset = (options, preset) => {
  if (options.globals && preset.globals) {
    options.globals = (0, _deepmerge().default)(preset.globals, options.globals);
  }
};
const setupPreset = async (options, optionsPreset) => {
  let preset;
  const presetPath = (0, _utils.replaceRootDirInPath)(options.rootDir, optionsPreset);
  const presetModule = _jestResolve().default.findNodeModule(presetPath.startsWith('.') || path().isAbsolute(presetPath) ? presetPath : `${presetPath}/${PRESET_NAME}`, {
    basedir: options.rootDir,
    extensions: PRESET_EXTENSIONS
  });
  try {
    if (!presetModule) {
      throw new Error(`Cannot find module '${presetPath}'`);
    }

    // Force re-evaluation to support multiple projects
    try {
      delete __webpack_require__.c[require.resolve(presetModule)];
    } catch {}
    preset = await (0, _jestUtil().requireOrImportModule)(presetModule);
  } catch (error) {
    if (error instanceof SyntaxError || error instanceof TypeError) {
      throw createConfigError(`  Preset ${_chalk().default.bold(presetPath)} is invalid:\n\n  ${error.message}\n  ${error.stack}`);
    }
    if (error.message.includes('Cannot find module')) {
      if (error.message.includes(presetPath)) {
        const preset = _jestResolve().default.findNodeModule(presetPath, {
          basedir: options.rootDir
        });
        if (preset) {
          throw createConfigError(`  Module ${_chalk().default.bold(presetPath)} should have "jest-preset.js" or "jest-preset.json" file at the root.`);
        }
        throw createConfigError(`  Preset ${_chalk().default.bold(presetPath)} not found relative to rootDir ${_chalk().default.bold(options.rootDir)}.`);
      }
      throw createConfigError(`  Missing dependency in ${_chalk().default.bold(presetPath)}:\n\n  ${error.message}\n  ${error.stack}`);
    }
    throw createConfigError(`  An unknown error occurred in ${_chalk().default.bold(presetPath)}:\n\n  ${error.message}\n  ${error.stack}`);
  }
  if (options.setupFiles) {
    options.setupFiles = [...(preset.setupFiles || []), ...options.setupFiles];
  }
  if (options.setupFilesAfterEnv) {
    options.setupFilesAfterEnv = [...(preset.setupFilesAfterEnv || []), ...options.setupFilesAfterEnv];
  }
  if (options.modulePathIgnorePatterns && preset.modulePathIgnorePatterns) {
    options.modulePathIgnorePatterns = [...preset.modulePathIgnorePatterns, ...options.modulePathIgnorePatterns];
  }
  mergeOptionWithPreset(options, preset, 'moduleNameMapper');
  mergeOptionWithPreset(options, preset, 'transform');
  mergeGlobalsWithPreset(options, preset);
  return {
    ...preset,
    ...options
  };
};
const setupBabelJest = options => {
  const transform = options.transform;
  let babelJest;
  if (transform) {
    const customJSPattern = Object.keys(transform).find(pattern => {
      const regex = new RegExp(pattern);
      return regex.test('a.js') || regex.test('a.jsx');
    });
    const customTSPattern = Object.keys(transform).find(pattern => {
      const regex = new RegExp(pattern);
      return regex.test('a.ts') || regex.test('a.tsx');
    });
    for (const pattern of [customJSPattern, customTSPattern]) {
      if (pattern) {
        const customTransformer = transform[pattern];
        if (Array.isArray(customTransformer)) {
          if (customTransformer[0] === 'babel-jest') {
            babelJest = require.resolve('babel-jest');
            customTransformer[0] = babelJest;
          } else if (customTransformer[0].includes('babel-jest')) {
            babelJest = customTransformer[0];
          }
        } else {
          if (customTransformer === 'babel-jest') {
            babelJest = require.resolve('babel-jest');
            transform[pattern] = babelJest;
          } else if (customTransformer.includes('babel-jest')) {
            babelJest = customTransformer;
          }
        }
      }
    }
  } else {
    babelJest = require.resolve('babel-jest');
    options.transform = {
      [_constants.DEFAULT_JS_PATTERN]: babelJest
    };
  }
};
const normalizeCollectCoverageFrom = (options, key) => {
  const initialCollectCoverageFrom = options[key];
  let value;
  if (!initialCollectCoverageFrom) {
    value = [];
  }
  if (Array.isArray(initialCollectCoverageFrom)) {
    value = initialCollectCoverageFrom;
  } else {
    try {
      value = JSON.parse(initialCollectCoverageFrom);
    } catch {}
    if (options[key] && !Array.isArray(value)) {
      value = [initialCollectCoverageFrom];
    }
  }
  if (value) {
    value = value.map(filePath => filePath.replace(/^(!?)(<rootDir>\/)(.*)/, '$1$3'));
  }
  return value;
};
const normalizeUnmockedModulePathPatterns = (options, key) =>
// _replaceRootDirTags is specifically well-suited for substituting
// <rootDir> in paths (it deals with properly interpreting relative path
// separators, etc).
//
// For patterns, direct global substitution is far more ideal, so we
// special case substitutions for patterns here.
options[key].map(pattern => (0, _jestRegexUtil().replacePathSepForRegex)(pattern.replaceAll('<rootDir>', options.rootDir)));
const normalizeMissingOptions = (options, configPath, projectIndex) => {
  if (!options.id) {
    options.id = (0, _nodeCrypto().createHash)('sha1').update(options.rootDir)
    // In case we load config from some path that has the same root dir
    .update(configPath || '').update(String(projectIndex)).digest('hex').slice(0, 32);
  }
  if (!options.setupFiles) {
    options.setupFiles = [];
  }
  return options;
};
const normalizeRootDir = options => {
  // Assert that there *is* a rootDir
  if (!options.rootDir) {
    throw createConfigError(`  Configuration option ${_chalk().default.bold('rootDir')} must be specified.`);
  }
  options.rootDir = path().normalize(options.rootDir);
  try {
    // try to resolve windows short paths, ignoring errors (permission errors, mostly)
    options.rootDir = (0, _jestUtil().tryRealpath)(options.rootDir);
  } catch {
    // ignored
  }
  verifyDirectoryExists(options.rootDir, 'rootDir');
  return {
    ...options,
    rootDir: options.rootDir
  };
};
const normalizeReporters = ({
  reporters,
  rootDir
}) => {
  if (!reporters || !Array.isArray(reporters)) {
    return undefined;
  }
  (0, _ReporterValidationErrors.validateReporters)(reporters);
  return reporters.map(reporterConfig => {
    const normalizedReporterConfig = typeof reporterConfig === 'string' ?
    // if reporter config is a string, we wrap it in an array
    // and pass an empty object for options argument, to normalize
    // the shape.
    [reporterConfig, {}] : reporterConfig;
    const reporterPath = (0, _utils.replaceRootDirInPath)(rootDir, normalizedReporterConfig[0]);
    if (!['agent', 'default', 'github-actions', 'summary'].includes(reporterPath)) {
      const reporter = _jestResolve().default.findNodeModule(reporterPath, {
        basedir: rootDir
      });
      if (!reporter) {
        throw new (_jestResolve().default.ModuleNotFoundError)('Could not resolve a module for a custom reporter.\n' + `  Module name: ${reporterPath}`);
      }
      normalizedReporterConfig[0] = reporter;
    }
    return normalizedReporterConfig;
  });
};
const buildTestPathPatterns = argv => {
  const patterns = [];
  if (argv._) {
    patterns.push(...argv._.map(x => x.toString()));
  }
  if (argv.testPathPatterns) {
    patterns.push(...argv.testPathPatterns);
  }
  const testPathPatterns = new (_pattern().TestPathPatterns)(patterns);
  if (!testPathPatterns.isValid()) {
    (0, _jestUtil().clearLine)(process.stdout);

    // eslint-disable-next-line no-console
    console.log(_chalk().default.red(`  Invalid testPattern ${testPathPatterns.toPretty()} supplied. ` + 'Running all tests instead.'));
    return new (_pattern().TestPathPatterns)([]);
  }
  return testPathPatterns;
};
function printConfig(opts) {
  const string = opts.map(ext => `'${ext}'`).join(', ');
  return _chalk().default.bold(`extensionsToTreatAsEsm: [${string}]`);
}
function validateExtensionsToTreatAsEsm(extensionsToTreatAsEsm) {
  if (!extensionsToTreatAsEsm || extensionsToTreatAsEsm.length === 0) {
    return;
  }
  const extensionWithoutDot = extensionsToTreatAsEsm.some(ext => !ext.startsWith('.'));
  if (extensionWithoutDot) {
    throw createConfigError(`  Option: ${printConfig(extensionsToTreatAsEsm)} includes a string that does not start with a period (${_chalk().default.bold('.')}).
  Please change your configuration to ${printConfig(extensionsToTreatAsEsm.map(ext => ext.startsWith('.') ? ext : `.${ext}`))}.`);
  }
  if (extensionsToTreatAsEsm.includes('.js')) {
    throw createConfigError(`  Option: ${printConfig(extensionsToTreatAsEsm)} includes ${_chalk().default.bold("'.js'")} which is always inferred based on ${_chalk().default.bold('type')} in its nearest ${_chalk().default.bold('package.json')}.`);
  }
  if (extensionsToTreatAsEsm.includes('.cjs')) {
    throw createConfigError(`  Option: ${printConfig(extensionsToTreatAsEsm)} includes ${_chalk().default.bold("'.cjs'")} which is always treated as CommonJS.`);
  }
  if (extensionsToTreatAsEsm.includes('.mjs')) {
    throw createConfigError(`  Option: ${printConfig(extensionsToTreatAsEsm)} includes ${_chalk().default.bold("'.mjs'")} which is always treated as an ECMAScript Module.`);
  }
}
async function normalize(initialOptions, argv, configPath, projectIndex = Number.POSITIVE_INFINITY, isProjectOptions) {
  const {
    hasDeprecationWarnings
  } = (0, _jestValidate().validate)(initialOptions, {
    comment: _utils.DOCUMENTATION_NOTE,
    deprecatedConfig: _Deprecated.default,
    exampleConfig: isProjectOptions ? _ValidConfig.initialProjectOptions : _ValidConfig.initialOptions,
    recursiveDenylist: [
    // 'coverageThreshold' allows to use 'global' and glob strings on the same
    // level, there's currently no way we can deal with such config
    'coverageThreshold', 'globals', 'moduleNameMapper', 'testEnvironmentOptions', 'transform'],
    ...(isProjectOptions && {
      unknown: unknownProjectOption
    })
  });
  let options = normalizeMissingOptions(normalizeRootDir((0, _setFromArgv.default)(initialOptions, argv)), configPath, projectIndex);
  if (options.preset) {
    options = await setupPreset(options, options.preset);
  }
  if (!options.setupFilesAfterEnv) {
    options.setupFilesAfterEnv = [];
  }
  options.testEnvironment = (0, _jestResolve().resolveTestEnvironment)({
    requireResolveFunction: requireResolve,
    rootDir: options.rootDir,
    testEnvironment: options.testEnvironment || require.resolve(_Defaults.default.testEnvironment)
  });
  if (!options.roots) {
    options.roots = [options.rootDir];
  }
  if (!options.testRunner || options.testRunner === 'circus' || options.testRunner === 'jest-circus' || options.testRunner === 'jest-circus/runner') {
    options.testRunner = require.resolve('jest-circus/runner');
  } else if (options.testRunner === 'jasmine2') {
    try {
      options.testRunner = require.resolve('jest-jasmine2');
    } catch (error) {
      if (error.code === 'MODULE_NOT_FOUND') {
        throw createConfigError('jest-jasmine is no longer shipped by default with Jest, you need to install it explicitly or provide an absolute path to Jest');
      }
      throw error;
    }
  }
  if (!options.coverageDirectory) {
    options.coverageDirectory = path().resolve(options.rootDir, 'coverage');
  }
  setupBabelJest(options);
  // TODO: Type this properly
  const newOptions = {
    ..._Defaults.default
  };
  if (options.resolver) {
    newOptions.resolver = (0, _utils.resolve)(null, {
      filePath: options.resolver,
      key: 'resolver',
      rootDir: options.rootDir
    });
  }
  validateExtensionsToTreatAsEsm(options.extensionsToTreatAsEsm);
  if (options.watchman == null) {
    options.watchman = _Defaults.default.watchman;
  }
  const optionKeys = Object.keys(options);
  optionKeys.reduce((newOptions, key) => {
    // The resolver has been resolved separately; skip it
    if (key === 'resolver') {
      return newOptions;
    }

    // This is cheating, because it claims that all keys of InitialOptions are Required.
    // We only really know it's Required for oldOptions[key], not for oldOptions.someOtherKey,
    // so oldOptions[key] is the only way it should be used.
    const oldOptions = options;
    let value;
    switch (key) {
      case 'setupFiles':
      case 'setupFilesAfterEnv':
      case 'snapshotSerializers':
        {
          const option = oldOptions[key];
          value = option && option.map(filePath => (0, _utils.resolve)(newOptions.resolver, {
            filePath,
            key,
            rootDir: options.rootDir
          }));
          break;
        }
      case 'modulePaths':
      case 'roots':
        {
          const option = oldOptions[key];
          value = option && option.map(filePath => path().resolve(options.rootDir, (0, _utils.replaceRootDirInPath)(options.rootDir, filePath)));
          break;
        }
      case 'collectCoverageFrom':
        value = normalizeCollectCoverageFrom(oldOptions, key);
        break;
      case 'cacheDirectory':
      case 'coverageDirectory':
        {
          const option = oldOptions[key];
          value = option && path().resolve(options.rootDir, (0, _utils.replaceRootDirInPath)(options.rootDir, option));
          break;
        }
      case 'dependencyExtractor':
      case 'globalSetup':
      case 'globalTeardown':
      case 'runtime':
      case 'snapshotResolver':
      case 'testResultsProcessor':
      case 'testRunner':
      case 'filter':
        {
          const option = oldOptions[key];
          value = option && (0, _utils.resolve)(newOptions.resolver, {
            filePath: option,
            key,
            rootDir: options.rootDir
          });
          break;
        }
      case 'runner':
        {
          const option = oldOptions[key];
          let runnerPath;
          if (Array.isArray(option)) {
            if (typeof option[0] !== 'string' || option[0].length === 0) {
              throw createConfigError('  Runner must be a string or a tuple [string, object].\n' + '  Configuration Documentation:\n' + '  https://jestjs.io/docs/configuration#runner-string--string-object');
            }
            runnerPath = option[0];
            const runnerOptions = option[1];
            if (runnerOptions != null && (typeof runnerOptions !== 'object' || Array.isArray(runnerOptions) || Object.getPrototypeOf(runnerOptions) !== Object.prototype)) {
              throw createConfigError('  Runner options must be a plain object.\n' + '  Configuration Documentation:\n' + '  https://jestjs.io/docs/configuration#runner-string--string-object');
            }
            newOptions.runnerOptions = runnerOptions ?? {};
          } else {
            runnerPath = option;
            newOptions.runnerOptions = {};
          }
          value = runnerPath && (0, _jestResolve().resolveRunner)(newOptions.resolver, {
            filePath: runnerPath,
            requireResolveFunction: requireResolve,
            rootDir: options.rootDir
          });
          break;
        }
      case 'prettierPath':
        {
          // We only want this to throw if "prettierPath" is explicitly passed
          // from config or CLI, and the requested path isn't found. Otherwise we
          // set it to null and throw an error lazily when it is used.

          const option = oldOptions[key];
          value = option && (0, _utils.resolve)(newOptions.resolver, {
            filePath: option,
            key,
            optional: option === _Defaults.default[key],
            rootDir: options.rootDir
          });
          break;
        }
      case 'moduleNameMapper':
        const moduleNameMapper = oldOptions[key];
        value = moduleNameMapper && Object.keys(moduleNameMapper).map(regex => {
          const item = moduleNameMapper && moduleNameMapper[regex];
          return item && [regex, (0, _utils._replaceRootDirTags)(options.rootDir, item)];
        });
        break;
      case 'transform':
        const transform = oldOptions[key];
        value = transform && Object.keys(transform).map(regex => {
          const transformElement = transform[regex];
          return [regex, (0, _utils.resolve)(newOptions.resolver, {
            filePath: Array.isArray(transformElement) ? transformElement[0] : transformElement,
            key,
            rootDir: options.rootDir
          }), Array.isArray(transformElement) ? transformElement[1] : {}];
        });
        break;
      case 'reporters':
        value = normalizeReporters(oldOptions);
        break;
      case 'coveragePathIgnorePatterns':
      case 'modulePathIgnorePatterns':
      case 'testPathIgnorePatterns':
      case 'transformIgnorePatterns':
      case 'watchPathIgnorePatterns':
      case 'unmockedModulePathPatterns':
        value = normalizeUnmockedModulePathPatterns(oldOptions, key);
        break;
      case 'haste':
        value = {
          ...oldOptions[key]
        };
        if (value.hasteImplModulePath != null) {
          const resolvedHasteImpl = (0, _utils.resolve)(newOptions.resolver, {
            filePath: (0, _utils.replaceRootDirInPath)(options.rootDir, value.hasteImplModulePath),
            key: 'haste.hasteImplModulePath',
            rootDir: options.rootDir
          });
          value.hasteImplModulePath = resolvedHasteImpl || undefined;
        }
        break;
      case 'projects':
        value = (oldOptions[key] || []).map(project => typeof project === 'string' ? (0, _utils._replaceRootDirTags)(options.rootDir, project) : project).reduce((projects, project) => {
          // Project can be specified as globs. If a glob matches any files,
          // We expand it to these paths. If not, we keep the original path
          // for the future resolution.
          const globMatches = typeof project === 'string' ? _glob().glob.sync(project, {
            windowsPathsNoEscape: true
          }) : [];
          const projectEntry = globMatches.length > 0 ? globMatches : project;
          return [...projects, ...(Array.isArray(projectEntry) ? projectEntry : [projectEntry])];
        }, []);
        break;
      case 'moduleDirectories':
      case 'testMatch':
        {
          const option = oldOptions[key];
          const rawValue = Array.isArray(option) || option == null ? option : [option];
          const replacedRootDirTags = (0, _utils._replaceRootDirTags)((0, _utils.escapeGlobCharacters)(options.rootDir), rawValue);
          if (replacedRootDirTags) {
            value = Array.isArray(replacedRootDirTags) ? replacedRootDirTags.map(_jestUtil().replacePathSepForGlob) : (0, _jestUtil().replacePathSepForGlob)(replacedRootDirTags);
          } else {
            value = replacedRootDirTags;
          }
          break;
        }
      case 'testRegex':
        {
          const option = oldOptions[key];
          value = option ? (Array.isArray(option) ? option : [option]).map(_jestRegexUtil().replacePathSepForRegex) : [];
          break;
        }
      case 'moduleFileExtensions':
        {
          value = oldOptions[key];
          if (Array.isArray(value) && (
          // If it's the wrong type, it can throw at a later time
          options.runner === undefined || options.runner === _Defaults.default.runner) &&
          // Only require 'js' for the default jest-runner
          !value.includes('js')) {
            const errorMessage = "  moduleFileExtensions must include 'js':\n" + '  but instead received:\n' + `    ${_chalk().default.bold.red(JSON.stringify(value))}`;

            // If `js` is not included, any dependency Jest itself injects into
            // the environment, like jasmine or sourcemap-support, will need to
            // `require` its modules with a file extension. This is not plausible
            // in the long run, so it's way easier to just fail hard early.
            // We might consider throwing if `json` is missing as well, as it's a
            // fair assumption from modules that they can do
            // `require('some-package/package') without the trailing `.json` as it
            // works in Node normally.
            throw createConfigError(`${errorMessage}\n  Please change your configuration to include 'js'.`);
          }
          break;
        }
      case 'bail':
        {
          const bail = oldOptions[key];
          if (typeof bail === 'boolean') {
            value = bail ? 1 : 0;
          } else if (typeof bail === 'string') {
            value = 1;
            // If Jest is invoked as `jest --bail someTestPattern` then need to
            // move the pattern from the `bail` configuration and into `argv._`
            // to be processed as an extra parameter
            argv._.push(bail);
          } else {
            value = oldOptions[key];
          }
          break;
        }
      case 'displayName':
        {
          const displayName = oldOptions[key];
          /**
           * Ensuring that displayName shape is correct here so that the
           * reporters can trust the shape of the data
           */
          if (typeof displayName === 'object') {
            const {
              name,
              color
            } = displayName;
            if (!name || !color || typeof name !== 'string' || typeof color !== 'string') {
              const errorMessage = `  Option "${_chalk().default.bold('displayName')}" must be of type:\n\n` + '  {\n' + '    name: string;\n' + '    color: string;\n' + '  }\n';
              throw createConfigError(errorMessage);
            }
            value = oldOptions[key];
          } else {
            value = {
              color: (0, _color.getDisplayNameColor)(Array.isArray(options.runner) ? options.runner[0] : options.runner),
              name: displayName
            };
          }
          break;
        }
      case 'testTimeout':
        {
          if (oldOptions[key] < 0) {
            throw createConfigError(`  Option "${_chalk().default.bold('testTimeout')}" must be a natural number.`);
          }
          value = oldOptions[key];
          break;
        }
      case 'snapshotFormat':
        {
          value = {
            ..._Defaults.default.snapshotFormat,
            ...oldOptions[key]
          };
          break;
        }
      case 'automock':
      case 'cache':
      case 'changedSince':
      case 'changedFilesWithAncestor':
      case 'clearMocks':
      case 'collectCoverage':
      case 'coverageProvider':
      case 'coverageReporters':
      case 'coverageThreshold':
      case 'detectLeaks':
      case 'detectOpenHandles':
      case 'errorOnDeprecated':
      case 'expand':
      case 'extensionsToTreatAsEsm':
      case 'globals':
      case 'fakeTimers':
      case 'findRelatedTests':
      case 'forceCoverageMatch':
      case 'forceExit':
      case 'injectGlobals':
      case 'lastCommit':
      case 'listTests':
      case 'logHeapUsage':
      case 'maxConcurrency':
      case 'id':
      case 'noStackTrace':
      case 'notify':
      case 'notifyMode':
      case 'onlyChanged':
      case 'onlyFailures':
      case 'openHandlesTimeout':
      case 'outputFile':
      case 'passWithNoTests':
      case 'randomize':
      case 'replname':
      case 'resetMocks':
      case 'resetModules':
      case 'restoreMocks':
      case 'rootDir':
      case 'runTestsByPath':
      case 'sandboxInjectedGlobals':
      case 'silent':
      case 'showSeed':
      case 'skipFilter':
      case 'skipNodeResolution':
      case 'slowTestThreshold':
      case 'testEnvironment':
      case 'testEnvironmentOptions':
      case 'testFailureExitCode':
      case 'testLocationInResults':
      case 'testNamePattern':
      case 'useStderr':
      case 'verbose':
      case 'waitForUnhandledRejections':
      case 'watch':
      case 'watchAll':
      case 'watchman':
      case 'workerGracefulExitTimeout':
      case 'workerThreads':
        value = oldOptions[key];
        break;
      case 'workerIdleMemoryLimit':
        value = (0, _stringToBytes.default)(oldOptions[key], (0, _nodeOs().totalmem)());
        break;
      case 'watchPlugins':
        value = (oldOptions[key] || []).map(watchPlugin => {
          if (typeof watchPlugin === 'string') {
            return {
              config: {},
              path: (0, _jestResolve().resolveWatchPlugin)(newOptions.resolver, {
                filePath: watchPlugin,
                requireResolveFunction: requireResolve,
                rootDir: options.rootDir
              })
            };
          } else {
            return {
              config: watchPlugin[1] || {},
              path: (0, _jestResolve().resolveWatchPlugin)(newOptions.resolver, {
                filePath: watchPlugin[0],
                requireResolveFunction: requireResolve,
                rootDir: options.rootDir
              })
            };
          }
        });
        break;
    }
    // @ts-expect-error: automock is missing in GlobalConfig, so what
    newOptions[key] = value;
    return newOptions;
  }, newOptions);
  if (options.watchman && options.haste?.enableSymlinks) {
    throw new (_jestValidate().ValidationError)('Validation Error', 'haste.enableSymlinks is incompatible with watchman', 'Either set haste.enableSymlinks to false or do not use watchman');
  }
  for (const [i, root] of newOptions.roots.entries()) {
    verifyDirectoryExists(root, `roots[${i}]`);
  }
  try {
    // try to resolve windows short paths, ignoring errors (permission errors, mostly)
    newOptions.cwd = (0, _jestUtil().tryRealpath)(process.cwd());
  } catch {
    // ignored
  }
  newOptions.testSequencer = (0, _jestResolve().resolveSequencer)(newOptions.resolver, {
    filePath: options.testSequencer || require.resolve(_Defaults.default.testSequencer),
    requireResolveFunction: requireResolve,
    rootDir: options.rootDir
  });
  if (newOptions.runner === _Defaults.default.runner) {
    newOptions.runner = require.resolve(newOptions.runner);
  }
  if (newOptions.runnerOptions == null) {
    newOptions.runnerOptions = {};
  }
  newOptions.nonFlagArgs = argv._?.map(arg => `${arg}`);
  const testPathPatterns = buildTestPathPatterns(argv);
  newOptions.testPathPatterns = testPathPatterns;
  newOptions.json = !!argv.json;
  newOptions.testFailureExitCode = Number.parseInt(newOptions.testFailureExitCode, 10);
  if (newOptions.lastCommit || newOptions.changedFilesWithAncestor || newOptions.changedSince) {
    newOptions.onlyChanged = true;
  }
  if (argv.all) {
    newOptions.onlyChanged = false;
    newOptions.onlyFailures = false;
  } else if (testPathPatterns.isSet()) {
    // When passing a test path pattern we don't want to only monitor changed
    // files unless `--watch` is also passed.
    newOptions.onlyChanged = newOptions.watch;
  }
  newOptions.randomize = newOptions.randomize || argv.randomize;
  newOptions.showSeed = newOptions.randomize || newOptions.showSeed || argv.showSeed;
  const upperBoundSeedValue = 2 ** 31;

  // bounds are determined by xoroshiro128plus which is used in v8 and is used here (at time of writing)
  newOptions.seed = argv.seed ?? Math.floor((2 ** 32 - 1) * Math.random() - upperBoundSeedValue);
  if (newOptions.seed < -upperBoundSeedValue || newOptions.seed > upperBoundSeedValue - 1) {
    throw new (_jestValidate().ValidationError)('Validation Error', `seed value must be between \`-0x80000000\` and \`0x7fffffff\` inclusive - instead it is ${newOptions.seed}`);
  }
  if (!newOptions.onlyChanged) {
    newOptions.onlyChanged = false;
  }
  if (!newOptions.lastCommit) {
    newOptions.lastCommit = false;
  }
  if (!newOptions.onlyFailures) {
    newOptions.onlyFailures = false;
  }
  if (!newOptions.watchAll) {
    newOptions.watchAll = false;
  }

  // as unknown since it can happen. We really need to fix the types here
  if (newOptions.moduleNameMapper === _Defaults.default.moduleNameMapper) {
    newOptions.moduleNameMapper = [];
  }
  if (argv.ci != null) {
    newOptions.ci = argv.ci;
  }
  newOptions.updateSnapshot = newOptions.ci && !argv.updateSnapshot ? 'none' : argv.updateSnapshot ? 'all' : 'new';
  newOptions.collectTests = argv.collectTests || false;
  newOptions.maxConcurrency = Number.parseInt(newOptions.maxConcurrency, 10);
  newOptions.maxWorkers = (0, _getMaxWorkers.default)(argv, options);
  newOptions.runInBand = argv.runInBand || false;
  if (newOptions.testRegex.length > 0 && options.testMatch) {
    throw createConfigError(`  Configuration options ${_chalk().default.bold('testMatch')} and` + ` ${_chalk().default.bold('testRegex')} cannot be used together.`);
  }
  if (newOptions.testRegex.length > 0 && !options.testMatch) {
    // Prevent the default testMatch conflicting with any explicitly
    // configured `testRegex` value
    newOptions.testMatch = [];
  }

  // If argv.json is set without an outputFile, coverageReporters shouldn't print
  // a text report to avoid polluting the JSON written to stdout.
  if (argv.json && !argv.outputFile) {
    newOptions.coverageReporters = (newOptions.coverageReporters || []).filter(reporter => reporter !== 'text');
  }

  // If collectCoverage is enabled while using --findRelatedTests we need to
  // avoid having false negatives in the generated coverage report.
  // The following: `--findRelatedTests '/rootDir/file1.js' --coverage`
  // Is transformed to: `--findRelatedTests '/rootDir/file1.js' --coverage --collectCoverageFrom 'file1.js'`
  // where arguments to `--collectCoverageFrom` should be globs (or relative
  // paths to the rootDir)
  if (newOptions.collectCoverage && argv.findRelatedTests) {
    let collectCoverageFrom = newOptions.nonFlagArgs.map(filename => {
      filename = (0, _utils.replaceRootDirInPath)(options.rootDir, filename);
      return path().isAbsolute(filename) ? path().relative(options.rootDir, filename) : filename;
    });

    // Don't override existing collectCoverageFrom options
    if (newOptions.collectCoverageFrom) {
      collectCoverageFrom = collectCoverageFrom.reduce((patterns, filename) => {
        if (!(0, _jestUtil().globsToMatcher)(newOptions.collectCoverageFrom)((0, _jestUtil().replacePathSepForGlob)(path().relative(options.rootDir, filename)))) {
          return patterns;
        }
        return [...patterns, filename];
      }, newOptions.collectCoverageFrom);
    }
    newOptions.collectCoverageFrom = collectCoverageFrom;
  } else if (!newOptions.collectCoverageFrom) {
    newOptions.collectCoverageFrom = [];
  }
  if (!newOptions.findRelatedTests) {
    newOptions.findRelatedTests = false;
  }
  if (!newOptions.projects) {
    newOptions.projects = [];
  }
  if (!newOptions.sandboxInjectedGlobals) {
    newOptions.sandboxInjectedGlobals = [];
  }
  if (!newOptions.forceExit) {
    newOptions.forceExit = false;
  }
  if (!newOptions.logHeapUsage) {
    newOptions.logHeapUsage = false;
  }
  if (argv.shard) {
    newOptions.shard = (0, _parseShardPair.parseShardPair)(argv.shard);
  }
  return {
    hasDeprecationWarnings,
    options: newOptions
  };
}

/***/ },

/***/ "./src/parseShardPair.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.parseShardPair = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const parseShardPair = pair => {
  const shardPair = pair.split('/').filter(d => /^\d+$/.test(d)).map(d => Number.parseInt(d, 10));
  const [shardIndex, shardCount] = shardPair;
  if (shardPair.length !== 2) {
    throw new Error('The shard option requires a string in the format of <n>/<m>.');
  }
  if (shardIndex === 0 || shardCount === 0) {
    throw new Error('The shard option requires 1-based values, received 0 or lower in the pair.');
  }
  if (shardIndex > shardCount) {
    throw new Error('The shard option <n>/<m> requires <n> to be lower or equal than <m>.');
  }
  return {
    shardCount,
    shardIndex
  };
};
exports.parseShardPair = parseShardPair;

/***/ },

/***/ "./src/readConfigFileAndSetRootDir.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = readConfigFileAndSetRootDir;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
function _parseJson() {
  const data = _interopRequireDefault(require("parse-json"));
  _parseJson = function () {
    return data;
  };
  return data;
}
function _stripJsonComments() {
  const data = _interopRequireDefault(require("strip-json-comments"));
  _stripJsonComments = function () {
    return data;
  };
  return data;
}
function _jestDocblock() {
  const data = require("jest-docblock");
  _jestDocblock = function () {
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
var _constants = __webpack_require__("./src/constants.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Read the configuration and set its `rootDir`
// 1. If it's a `package.json` file, we look into its "jest" property
// 2. If it's a `jest.config.ts`/`jest.config.cts`/`jest.config.mts` file,
//    we use native TypeScript support when possible, otherwise a TS loader.
// 3. For any other file, we just require it. If we receive an 'ERR_REQUIRE_ESM'
//    from node, perform a dynamic import instead.
async function readConfigFileAndSetRootDir(configPath) {
  const isTS = configPath.endsWith(_constants.JEST_CONFIG_EXT_TS) || configPath.endsWith(_constants.JEST_CONFIG_EXT_MTS) || configPath.endsWith(_constants.JEST_CONFIG_EXT_CTS);
  const isMTS = configPath.endsWith(_constants.JEST_CONFIG_EXT_MTS);
  const isJSON = configPath.endsWith(_constants.JEST_CONFIG_EXT_JSON);
  let configObject;
  try {
    if (isTS) {
      // .mts is always ESM, so attempt import-based loading first.
      // @ts-expect-error: Type assertion can be removed once @types/node is updated to 23 https://nodejs.org/api/process.html#processfeaturestypescript
      if (isMTS || process.features.typescript) {
        try {
          // Try native node TypeScript support first.
          configObject = await (0, _jestUtil().requireOrImportModule)(configPath);
        } catch (requireOrImportModuleError) {
          if (isMTS) {
            // .mts is always ESM and cannot be loaded via require()/ts-node.
            throw new Error('jest.config.mts requires native TypeScript support. Ensure you are using Node.js 22.18+ or 23.6+.', {
              cause: requireOrImportModuleError
            });
          }
          if (!(requireOrImportModuleError instanceof SyntaxError)) {
            if (!hasTsLoaderExplicitlyConfigured(configPath)) {
              throw requireOrImportModuleError;
            }
          }
          try {
            // There are various reasons of failed loadout of Jest config in Typescript:
            // 1. User has specified a TypeScript loader in the docblock and
            // desire non-native compilation (https://github.com/jestjs/jest/issues/15837)
            // 2. Likely ESM in a file interpreted as CJS, which means it needs to be
            // compiled. We ignore the error and try to load it with a loader.
            configObject = await loadTSConfigFile(configPath);
          } catch (loadTSConfigFileError) {
            // If we still encounter an error, we throw both messages combined.
            // This string is caught further down and merged into a new error message.
            // eslint-disable-next-line no-throw-literal
            throw (
              // Preamble text is added further down:
              // Jest: Failed to parse the TypeScript config file ${configPath}\n
              '  both with the native node TypeScript support and configured TypeScript loaders.\n' + '    Errors were:\n' + `    - ${requireOrImportModuleError}\n` + `    - ${loadTSConfigFileError}`
            );
          }
        }
      } else {
        configObject = await loadTSConfigFile(configPath);
      }
    } else if (isJSON) {
      const fileContent = fs().readFileSync(configPath, 'utf8');
      configObject = (0, _parseJson().default)((0, _stripJsonComments().default)(fileContent), configPath);
    } else {
      configObject = await (0, _jestUtil().requireOrImportModule)(configPath);
    }
  } catch (error) {
    if (isTS) {
      throw new Error(`Jest: Failed to parse the TypeScript config file ${configPath}\n` + `  ${error}`);
    }
    throw error;
  }
  if (configPath.endsWith(_constants.PACKAGE_JSON)) {
    // Event if there's no "jest" property in package.json we will still use
    // an empty object.
    configObject = configObject.jest || {};
  }
  if (typeof configObject === 'function') {
    configObject = await configObject();
  }
  if (configObject.rootDir) {
    // We don't touch it if it has an absolute path specified
    if (!path().isAbsolute(configObject.rootDir)) {
      // otherwise, we'll resolve it relative to the file's __dirname
      configObject = {
        ...configObject,
        rootDir: path().resolve(path().dirname(configPath), configObject.rootDir)
      };
    }
  } else {
    // If rootDir is not there, we'll set it to this file's __dirname
    configObject = {
      ...configObject,
      rootDir: path().dirname(configPath)
    };
  }
  return configObject;
}

// Load the TypeScript configuration
let extraTSLoaderOptions;
const hasTsLoaderExplicitlyConfigured = configPath => {
  const docblockPragmas = loadDocblockPragmasInConfig(configPath);
  const tsLoader = docblockPragmas['jest-config-loader'];
  return !Array.isArray(tsLoader) && (tsLoader ?? '').trim() !== '';
};
const loadDocblockPragmasInConfig = configPath => {
  const docblockPragmas = (0, _jestDocblock().parse)((0, _jestDocblock().extract)(fs().readFileSync(configPath, 'utf8')));
  return docblockPragmas;
};
const loadTSConfigFile = async configPath => {
  // Get registered TypeScript compiler instance
  const docblockPragmas = loadDocblockPragmasInConfig(configPath);
  const tsLoader = docblockPragmas['jest-config-loader'] || 'ts-node';
  const docblockTSLoaderOptions = docblockPragmas['jest-config-loader-options'];
  if (typeof docblockTSLoaderOptions === 'string') {
    extraTSLoaderOptions = JSON.parse(docblockTSLoaderOptions);
  }
  if (Array.isArray(tsLoader)) {
    throw new TypeError(`Jest: You can only define a single loader through docblocks, got "${tsLoader.join(', ')}"`);
  }
  const registeredCompiler = await getRegisteredCompiler(tsLoader);
  registeredCompiler.enabled(true);
  let configObject = (0, _jestUtil().interopRequireDefault)(require(configPath)).default;

  // In case the config is a function which imports more Typescript code
  if (typeof configObject === 'function') {
    configObject = await configObject();
  }
  registeredCompiler.enabled(false);
  return configObject;
};
let registeredCompilerPromise;
function getRegisteredCompiler(loader) {
  // Cache the promise to avoid multiple registrations
  registeredCompilerPromise = registeredCompilerPromise ?? registerTsLoader(loader);
  return registeredCompilerPromise;
}
async function registerTsLoader(loader) {
  try {
    // Register TypeScript compiler instance
    if (loader === 'ts-node') {
      const tsLoader = await import(/* webpackIgnore: true */'ts-node');
      return tsLoader.register({
        compilerOptions: {
          module: 'CommonJS'
        },
        moduleTypes: {
          '**': 'cjs'
        },
        ...extraTSLoaderOptions
      });
    } else if (loader === 'esbuild-register') {
      const tsLoader = await import(/* webpackIgnore: true */'esbuild-register/dist/node');
      let instance;
      return {
        enabled: bool => {
          if (bool) {
            instance = tsLoader.register({
              target: `node${process.version.slice(1)}`,
              ...extraTSLoaderOptions
            });
          } else {
            instance?.unregister();
          }
        }
      };
    }
    throw new Error(`Jest: '${loader}' is not a valid TypeScript configuration loader.`);
  } catch (error) {
    if ((0, _jestUtil().isError)(error) && error.code === 'ERR_MODULE_NOT_FOUND') {
      throw new Error(`Jest: '${loader}' is required for the TypeScript configuration files. Make sure it is installed\nError: ${error.message}`, {
        cause: error
      });
    }
    throw error;
  }
}

/***/ },

/***/ "./src/resolveConfigPath.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = resolveConfigPath;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
function _slash() {
  const data = _interopRequireDefault(require("slash"));
  _slash = function () {
    return data;
  };
  return data;
}
function _jestValidate() {
  const data = require("jest-validate");
  _jestValidate = function () {
    return data;
  };
  return data;
}
var _constants = __webpack_require__("./src/constants.ts");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const isFile = filePath => fs().existsSync(filePath) && !fs().lstatSync(filePath).isDirectory();
const getConfigFilename = ext => _constants.JEST_CONFIG_BASE_NAME + ext;
function resolveConfigPath(pathToResolve, cwd, skipMultipleConfigError = false) {
  if (!path().isAbsolute(cwd)) {
    throw new Error(`"cwd" must be an absolute path. cwd: ${cwd}`);
  }
  const absolutePath = path().isAbsolute(pathToResolve) ? pathToResolve : path().resolve(cwd, pathToResolve);
  if (isFile(absolutePath)) {
    return absolutePath;
  }

  // This is a guard against passing non existing path as a project/config,
  // that will otherwise result in a very confusing situation.
  // e.g.
  // With a directory structure like this:
  //   my_project/
  //     package.json
  //
  // Passing a `my_project/some_directory_that_doesnt_exist` as a project
  // name will resolve into a (possibly empty) `my_project/package.json` and
  // try to run all tests it finds under `my_project` directory.
  if (!fs().existsSync(absolutePath)) {
    throw new Error("Can't find a root directory while resolving a config file path.\n" + `Provided path to resolve: ${pathToResolve}\n` + `cwd: ${cwd}`);
  }
  return resolveConfigPathByTraversing(absolutePath, pathToResolve, cwd, skipMultipleConfigError);
}
const resolveConfigPathByTraversing = (pathToResolve, initialPath, cwd, skipMultipleConfigError) => {
  const configFiles = _constants.JEST_CONFIG_EXT_ORDER.map(ext => path().resolve(pathToResolve, getConfigFilename(ext))).filter(isFile);
  const packageJson = findPackageJson(pathToResolve);
  if (packageJson) {
    const jestKey = getPackageJsonJestKey(packageJson);
    if (jestKey) {
      if (typeof jestKey === 'string') {
        const absolutePath = path().isAbsolute(jestKey) ? jestKey : path().resolve(pathToResolve, jestKey);
        if (!isFile(absolutePath)) {
          throw new (_jestValidate().ValidationError)(`${_utils.BULLET}Validation Error`, `  Configuration in ${_chalk().default.bold(packageJson)} is not valid. ` + `Jest expects the string configuration to point to a file, but ${absolutePath} is not. ` + `Please check your Jest configuration in ${_chalk().default.bold(packageJson)}.`, _utils.DOCUMENTATION_NOTE);
        }
        configFiles.push(absolutePath);
      } else {
        configFiles.push(packageJson);
      }
    }
  }
  if (!skipMultipleConfigError && configFiles.length > 1) {
    throw new (_jestValidate().ValidationError)(...makeMultipleConfigsErrorMessage(configFiles));
  }
  if (configFiles.length > 0 || packageJson) {
    return configFiles[0] ?? packageJson;
  }

  // This is the system root.
  // We tried everything, config is nowhere to be found ¯\_(ツ)_/¯
  if (pathToResolve === path().dirname(pathToResolve)) {
    throw new Error(makeResolutionErrorMessage(initialPath, cwd));
  }

  // go up a level and try it again
  return resolveConfigPathByTraversing(path().dirname(pathToResolve), initialPath, cwd, skipMultipleConfigError);
};
const findPackageJson = pathToResolve => {
  const packagePath = path().resolve(pathToResolve, _constants.PACKAGE_JSON);
  if (isFile(packagePath)) {
    return packagePath;
  }
  return undefined;
};
const getPackageJsonJestKey = packagePath => {
  try {
    const content = fs().readFileSync(packagePath, 'utf8');
    const parsedContent = JSON.parse(content);
    if ('jest' in parsedContent) {
      return parsedContent.jest;
    }
  } catch {}
  return undefined;
};
const makeResolutionErrorMessage = (initialPath, cwd) => 'Could not find a config file based on provided values:\n' + `path: "${initialPath}"\n` + `cwd: "${cwd}"\n` + 'Config paths must be specified by either a direct path to a config\n' + 'file, or a path to a directory. If directory is given, Jest will try to\n' + `traverse directory tree up, until it finds one of those files in exact order: ${_constants.JEST_CONFIG_EXT_ORDER.map(ext => `"${getConfigFilename(ext)}"`).join(' or ')}.`;
function extraIfPackageJson(configPath) {
  if (configPath.endsWith(_constants.PACKAGE_JSON)) {
    return '`jest` key in ';
  }
  return '';
}
const makeMultipleConfigsErrorMessage = configPaths => [`${_utils.BULLET}${_chalk().default.bold('Multiple configurations found')}`, [...configPaths.map(configPath => `    * ${extraIfPackageJson(configPath)}${(0, _slash().default)(configPath)}`), '', '  Implicit config resolution does not allow multiple configuration files.', '  Either remove unused config files or select one explicitly with `--config`.'].join('\n'), _utils.DOCUMENTATION_NOTE];

/***/ },

/***/ "./src/setFromArgv.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = setFromArgv;
var _utils = __webpack_require__("./src/utils.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const specialArgs = new Set(['_', '$0', 'h', 'help', 'config']);
function setFromArgv(options, argv) {
  const argvToOptions = Object.keys(argv).reduce((options, key) => {
    if (argv[key] === undefined || specialArgs.has(key)) {
      return options;
    }
    switch (key) {
      case 'coverage':
        options.collectCoverage = argv[key];
        break;
      case 'json':
        options.useStderr = argv[key];
        break;
      case 'watchAll':
        options.watch = false;
        options.watchAll = argv[key];
        break;
      case 'env':
        options.testEnvironment = argv[key];
        break;
      case 'config':
        break;
      case 'coverageThreshold':
      case 'globals':
      case 'haste':
      case 'moduleNameMapper':
      case 'testEnvironmentOptions':
      case 'transform':
        const str = argv[key];
        if ((0, _utils.isJSONString)(str)) {
          options[key] = JSON.parse(str);
        }
        break;
      default:
        options[key] = argv[key];
    }
    return options;
  }, {});
  return {
    ...options,
    ...((0, _utils.isJSONString)(argv.config) ? JSON.parse(argv.config) : null),
    ...argvToOptions
  };
}

/***/ },

/***/ "./src/stringToBytes.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/**
 * Converts a string representing an amount of memory to bytes.
 *
 * @param input The value to convert to bytes.
 * @param percentageReference The reference value to use when a '%' value is supplied.
 */
function stringToBytes(input, percentageReference) {
  if (input === null || input === undefined) {
    return input;
  }
  if (typeof input === 'string') {
    if (Number.isNaN(Number.parseFloat(input.slice(-1)))) {
      // eslint-disable-next-line prefer-const
      let [, numericString, trailingChars] = input.match(/(.*?)([^\d.-]+)$/i) || [];
      if (trailingChars && numericString) {
        const numericValue = Number.parseFloat(numericString);
        trailingChars = trailingChars.toLowerCase();
        switch (trailingChars) {
          case '%':
            input = numericValue / 100;
            break;
          case 'kb':
          case 'k':
            return numericValue * 1000;
          case 'kib':
            return numericValue * 1024;
          case 'mb':
          case 'm':
            return numericValue * 1000 * 1000;
          case 'mib':
            return numericValue * 1024 * 1024;
          case 'gb':
          case 'g':
            return numericValue * 1000 * 1000 * 1000;
          case 'gib':
            return numericValue * 1024 * 1024 * 1024;
        }
      }

      // It ends in some kind of char so we need to do some parsing
    } else {
      input = Number.parseFloat(input);
    }
  }
  if (typeof input === 'number') {
    if (input === 0) {
      return 0;
    } else if (input <= 1 && input > 0) {
      if (percentageReference) {
        return Math.floor(input * percentageReference);
      } else {
        throw new Error('For a percentage based memory limit a percentageReference must be supplied');
      }
    } else if (input > 1) {
      return Math.floor(input);
    } else {
      throw new Error('Unexpected numerical input');
    }
  }
  throw new Error('Unexpected input');
}

// https://github.com/import-js/eslint-plugin-import/issues/1590
var _default = exports["default"] = stringToBytes;

/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.resolve = exports.replaceRootDirInPath = exports.isJSONString = exports.escapeGlobCharacters = exports._replaceRootDirTags = exports.DOCUMENTATION_NOTE = exports.BULLET = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
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
function _jestResolve() {
  const data = _interopRequireDefault(require("jest-resolve"));
  _jestResolve = function () {
    return data;
  };
  return data;
}
function _jestValidate() {
  const data = require("jest-validate");
  _jestValidate = function () {
    return data;
  };
  return data;
}
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const BULLET = exports.BULLET = _chalk().default.bold('\u25CF ');
const DOCUMENTATION_NOTE = exports.DOCUMENTATION_NOTE = `  ${_chalk().default.bold('Configuration Documentation:')}
  https://jestjs.io/docs/configuration
`;
const createValidationError = message => new (_jestValidate().ValidationError)(`${BULLET}Validation Error`, message, DOCUMENTATION_NOTE);
const resolve = (resolver, {
  key,
  filePath,
  rootDir,
  optional
}) => {
  const module = _jestResolve().default.findNodeModule(replaceRootDirInPath(rootDir, filePath), {
    basedir: rootDir,
    resolver: resolver || undefined
  });
  if (!module && !optional) {
    throw createValidationError(`  Module ${_chalk().default.bold(filePath)} in the ${_chalk().default.bold(key)} option was not found.
         ${_chalk().default.bold('<rootDir>')} is: ${rootDir}`);
  }
  /// can cast as string since nulls will be thrown
  return module;
};
exports.resolve = resolve;
const escapeGlobCharacters = path => path.replaceAll(/([!()*?[\\\]{}])/g, '\\$1');
exports.escapeGlobCharacters = escapeGlobCharacters;
const replaceRootDirInPath = (rootDir, filePath) => {
  if (!filePath.startsWith('<rootDir>')) {
    return filePath;
  }
  return path().resolve(rootDir, path().normalize(`./${filePath.slice('<rootDir>'.length)}`));
};
exports.replaceRootDirInPath = replaceRootDirInPath;
const _replaceRootDirInObject = (rootDir, config) => {
  const newConfig = {};
  for (const configKey in config) {
    newConfig[configKey] = configKey === 'rootDir' ? config[configKey] : _replaceRootDirTags(rootDir, config[configKey]);
  }
  return newConfig;
};
const _replaceRootDirTags = (rootDir, config) => {
  if (config == null) {
    return config;
  }
  switch (typeof config) {
    case 'object':
      if (Array.isArray(config)) {
        /// can be string[] or {}[]
        return config.map(item => _replaceRootDirTags(rootDir, item));
      }
      if (config instanceof RegExp) {
        return config;
      }
      return _replaceRootDirInObject(rootDir, config);
    case 'string':
      return replaceRootDirInPath(rootDir, config);
  }
  return config;
};
exports._replaceRootDirTags = _replaceRootDirTags;
// newtype
const isJSONString = text => text != null && typeof text === 'string' && text.startsWith('{') && text.endsWith('}');
exports.isJSONString = isJSONString;

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
/******/ 			id: moduleId,
/******/ 			loaded: false,
/******/ 			exports: {}
/******/ 		};
/******/ 	
/******/ 		// Execute the module function
/******/ 		__webpack_modules__[moduleId](module, module.exports, __webpack_require__);
/******/ 	
/******/ 		// Flag the module as loaded
/******/ 		module.loaded = true;
/******/ 	
/******/ 		// Return the exports of the module
/******/ 		return module.exports;
/******/ 	}
/******/ 	
/******/ 	// expose the module cache
/******/ 	__webpack_require__.c = __webpack_module_cache__;
/******/ 	
/************************************************************************/
/******/ 	
/******/ 	// module cache are used so entry inlining is disabled
/******/ 	// startup
/******/ 	// Load entry module and return exports
/******/ 	var __webpack_exports__ = __webpack_require__("./src/index.ts");
/******/ 	module.exports = __webpack_exports__;
/******/ 	
/******/ })()
;