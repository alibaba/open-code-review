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

/***/ "./src/args.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.check = check;
exports.usage = exports.options = exports.docs = void 0;
function _jestConfig() {
  const data = require("jest-config");
  _jestConfig = function () {
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

function check(argv) {
  if (argv.runInBand && Object.prototype.hasOwnProperty.call(argv, 'maxWorkers')) {
    throw new Error('Both --runInBand and --maxWorkers were specified, only one is allowed.');
  }
  for (const key of ['onlyChanged', 'lastCommit', 'changedFilesWithAncestor', 'changedSince']) {
    if (argv[key] && argv.watchAll) {
      throw new Error(`Both --${key} and --watchAll were specified, but cannot be used ` + 'together. Try the --watch option which reruns only tests ' + 'related to changed files.');
    }
  }
  if (argv.onlyFailures && argv.watchAll) {
    throw new Error('Both --onlyFailures and --watchAll were specified, only one is allowed.');
  }
  if (argv.findRelatedTests && argv._.length === 0) {
    throw new Error('The --findRelatedTests option requires file paths to be specified.\n' + 'Example usage: jest --findRelatedTests ./src/source.js ' + './src/index.js.');
  }
  if (Object.prototype.hasOwnProperty.call(argv, 'maxWorkers') && argv.maxWorkers === undefined) {
    throw new Error('The --maxWorkers (-w) option requires a number or string to be specified.\n' + 'Example usage: jest --maxWorkers 2\n' + 'Example usage: jest --maxWorkers 50%\n' + 'Or did you mean --watch?');
  }
  if (argv.selectProjects && argv.selectProjects.length === 0) {
    throw new Error('The --selectProjects option requires the name of at least one project to be specified.\n' + 'Example usage: jest --selectProjects my-first-project my-second-project');
  }
  if (argv.ignoreProjects && argv.ignoreProjects.length === 0) {
    throw new Error('The --ignoreProjects option requires the name of at least one project to be specified.\n' + 'Example usage: jest --ignoreProjects my-first-project my-second-project');
  }
  if (argv.config && !(0, _jestConfig().isJSONString)(argv.config) && !new RegExp(`\\.(${_jestConfig().constants.JEST_CONFIG_EXT_ORDER.map(e => e.slice(1)).join('|')})$`, 'i').test(argv.config)) {
    throw new Error(`The --config option requires a JSON string literal, or a file path with one of these extensions: ${_jestConfig().constants.JEST_CONFIG_EXT_ORDER.join(', ')}.\nExample usage: jest --config ./jest.config.js`);
  }
  return true;
}
const usage = exports.usage = 'Usage: $0 [--config=<pathToConfigFile>] [TestPathPatterns]';
const docs = exports.docs = 'Documentation: https://jestjs.io/docs/cli';

// The default values are all set in jest-config
const options = exports.options = {
  all: {
    description: 'The opposite of `onlyChanged`. If `onlyChanged` is set by ' + 'default, running jest with `--all` will force Jest to run all tests ' + 'instead of running only tests related to changed files.',
    type: 'boolean'
  },
  automock: {
    description: 'Automock all files by default.',
    type: 'boolean'
  },
  bail: {
    alias: 'b',
    description: 'Exit the test suite immediately after `n` number of failing tests.',
    type: 'boolean'
  },
  cache: {
    description: 'Whether to use the transform cache. Disable the cache ' + 'using --no-cache.',
    type: 'boolean'
  },
  cacheDirectory: {
    description: 'The directory where Jest should store its cached ' + ' dependency information.',
    requiresArg: true,
    type: 'string'
  },
  changedFilesWithAncestor: {
    description: 'Runs tests related to the current changes and the changes made in the ' + 'last commit. Behaves similarly to `--onlyChanged`.',
    type: 'boolean'
  },
  changedSince: {
    description: 'Runs tests related to the changes since the provided branch. If the ' + 'current branch has diverged from the given branch, then only changes ' + 'made locally will be tested. Behaves similarly to `--onlyChanged`.',
    requiresArg: true,
    type: 'string'
  },
  ci: {
    description: 'Whether to run Jest in continuous integration (CI) mode. ' + 'This option is on by default in most popular CI environments. It will ' + 'prevent snapshots from being written unless explicitly requested.',
    type: 'boolean'
  },
  clearCache: {
    description: 'Clears the configured Jest cache directory and then exits. ' + 'Default directory can be found by calling jest --showConfig',
    type: 'boolean'
  },
  clearMocks: {
    description: 'Automatically clear mock calls, instances, contexts and results before every test. ' + 'Equivalent to calling jest.clearAllMocks() before each test.',
    type: 'boolean'
  },
  collectCoverage: {
    description: 'Alias for --coverage.',
    type: 'boolean'
  },
  collectCoverageFrom: {
    description: 'A glob pattern relative to <rootDir> matching the files that coverage ' + 'info needs to be collected from.',
    requiresArg: true,
    type: 'string'
  },
  collectTests: {
    description: 'Discover and report all test cases without executing them. ' + 'Prints a tree of test suites and test names.',
    type: 'boolean'
  },
  color: {
    description: 'Forces test results output color highlighting (even if ' + 'stdout is not a TTY). Set to false if you would like to have no colors.',
    type: 'boolean'
  },
  colors: {
    description: 'Alias for `--color`.',
    type: 'boolean'
  },
  config: {
    alias: 'c',
    description: 'The path to a jest config file specifying how to find ' + 'and execute tests. If no rootDir is set in the config, the directory ' + 'containing the config file is assumed to be the rootDir for the project. ' + 'This can also be a JSON encoded value which Jest will use as configuration.',
    requiresArg: true,
    type: 'string'
  },
  coverage: {
    description: 'Indicates that test coverage information should be ' + 'collected and reported in the output.',
    type: 'boolean'
  },
  coverageDirectory: {
    description: 'The directory where Jest should output its coverage files.',
    requiresArg: true,
    type: 'string'
  },
  coveragePathIgnorePatterns: {
    description: 'An array of regexp pattern strings that are matched ' + 'against all file paths before executing the test. If the file path ' + 'matches any of the patterns, coverage information will be skipped.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  coverageProvider: {
    choices: ['babel', 'v8'],
    description: 'Select between Babel and V8 to collect coverage',
    requiresArg: true
  },
  coverageReporters: {
    description: 'A list of reporter names that Jest uses when writing ' + 'coverage reports. Any istanbul reporter can be used.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  coverageThreshold: {
    description: 'A JSON string with which will be used to configure ' + 'minimum threshold enforcement for coverage results',
    requiresArg: true,
    type: 'string'
  },
  debug: {
    description: 'Print debugging info about your jest config.',
    type: 'boolean'
  },
  detectLeaks: {
    description: '**EXPERIMENTAL**: Detect memory leaks in tests. After executing a ' + 'test, it will try to garbage collect the global object used, and fail ' + 'if it was leaked',
    type: 'boolean'
  },
  detectOpenHandles: {
    description: 'Print out remaining open handles preventing Jest from exiting at the ' + 'end of a test run. Implies `runInBand`.',
    type: 'boolean'
  },
  errorOnDeprecated: {
    description: 'Make calling deprecated APIs throw helpful error messages.',
    type: 'boolean'
  },
  expand: {
    alias: 'e',
    description: 'Use this flag to show full diffs instead of a patch.',
    type: 'boolean'
  },
  filter: {
    description: 'Path to a module exporting a filtering function. This method receives ' + 'a list of tests which can be manipulated to exclude tests from ' + 'running. Especially useful when used in conjunction with a testing ' + 'infrastructure to filter known broken tests.',
    requiresArg: true,
    type: 'string'
  },
  findRelatedTests: {
    description: 'Find related tests for a list of source files that were ' + 'passed in as arguments. Useful for pre-commit hook integration to run ' + 'the minimal amount of tests necessary.',
    type: 'boolean'
  },
  forceExit: {
    description: 'Force Jest to exit after all tests have completed running. ' + 'This is useful when resources set up by test code cannot be ' + 'adequately cleaned up.',
    type: 'boolean'
  },
  globalSetup: {
    description: 'The path to a module that runs before All Tests.',
    requiresArg: true,
    type: 'string'
  },
  globalTeardown: {
    description: 'The path to a module that runs after All Tests.',
    requiresArg: true,
    type: 'string'
  },
  globals: {
    description: 'A JSON string with map of global variables that need ' + 'to be available in all test environments.',
    requiresArg: true,
    type: 'string'
  },
  haste: {
    description: 'A JSON string with map of variables for the haste module system',
    requiresArg: true,
    type: 'string'
  },
  ignoreProjects: {
    description: 'Ignore the tests of the specified projects. ' + 'Jest uses the attribute `displayName` in the configuration to identify each project.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  injectGlobals: {
    description: 'Should Jest inject global variables or not',
    type: 'boolean'
  },
  json: {
    description: 'Prints the test results in JSON. This mode will send all ' + 'other test output and user messages to stderr.',
    type: 'boolean'
  },
  lastCommit: {
    description: 'Run all tests affected by file changes in the last commit made. ' + 'Behaves similarly to `--onlyChanged`.',
    type: 'boolean'
  },
  listTests: {
    description: 'Lists all tests Jest will run given the arguments and ' + 'exits. Most useful in a CI system together with `--findRelatedTests` ' + 'to determine the tests Jest will run based on specific files',
    type: 'boolean'
  },
  logHeapUsage: {
    description: 'Logs the heap usage after every test. Useful to debug ' + 'memory leaks. Use together with `--runInBand` and `--expose-gc` in ' + 'node.',
    type: 'boolean'
  },
  maxConcurrency: {
    description: 'Specifies the maximum number of tests that are allowed to run ' + 'concurrently. This only affects tests using `test.concurrent`.',
    requiresArg: true,
    type: 'number'
  },
  maxWorkers: {
    alias: 'w',
    description: 'Specifies the maximum number of workers the worker-pool ' + 'will spawn for running tests. This defaults to the number of the ' + 'cores available on your machine. (its usually best not to override ' + 'this default)',
    requiresArg: true,
    type: 'string'
  },
  moduleDirectories: {
    description: 'An array of directory names to be searched recursively ' + "up from the requiring module's location.",
    requiresArg: true,
    string: true,
    type: 'array'
  },
  moduleFileExtensions: {
    description: 'An array of file extensions your modules use. If you ' + 'require modules without specifying a file extension, these are the ' + 'extensions Jest will look for.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  moduleNameMapper: {
    description: 'A JSON string with a map from regular expressions to ' + 'module names or to arrays of module names that allow to stub ' + 'out resources, like images or styles with a single module',
    requiresArg: true,
    type: 'string'
  },
  modulePathIgnorePatterns: {
    description: 'An array of regexp pattern strings that are matched ' + 'against all module paths before those paths are to be considered ' + '"visible" to the module loader.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  modulePaths: {
    description: 'An alternative API to setting the NODE_PATH env variable, ' + 'modulePaths is an array of absolute paths to additional locations to ' + 'search when resolving modules.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  noStackTrace: {
    description: 'Disables stack trace in test results output',
    type: 'boolean'
  },
  notify: {
    description: 'Activates notifications for test results.',
    type: 'boolean'
  },
  notifyMode: {
    choices: ['always', 'failure', 'success', 'change', 'success-change', 'failure-change'],
    description: 'Specifies when notifications will appear for test results.',
    requiresArg: true
  },
  onlyChanged: {
    alias: 'o',
    description: 'Attempts to identify which tests to run based on which ' + "files have changed in the current repository. Only works if you're " + 'running tests in a git or hg repository at the moment.',
    type: 'boolean'
  },
  onlyFailures: {
    alias: 'f',
    description: 'Run tests that failed in the previous execution.',
    type: 'boolean'
  },
  openHandlesTimeout: {
    description: 'Print a warning about probable open handles if Jest does not exit ' + 'cleanly after this number of milliseconds. `0` to disable.',
    requiresArg: true,
    type: 'number'
  },
  outputFile: {
    description: 'Write test results to a file when the --json option is ' + 'also specified.',
    requiresArg: true,
    type: 'string'
  },
  passWithNoTests: {
    description: 'Will not fail if no tests are found (for example while using `--testPathPatterns`.)',
    type: 'boolean'
  },
  preset: {
    description: "A preset that is used as a base for Jest's configuration.",
    requiresArg: true,
    type: 'string'
  },
  prettierPath: {
    description: 'The path to the "prettier" module used for inline snapshots.',
    requiresArg: true,
    type: 'string'
  },
  projects: {
    description: 'A list of projects that use Jest to run all tests of all ' + 'projects in a single instance of Jest.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  randomize: {
    description: 'Shuffle the order of the tests within a file. In order to choose the seed refer to the `--seed` CLI option.',
    type: 'boolean'
  },
  reporters: {
    description: 'A list of custom reporters for the test suite.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  resetMocks: {
    description: 'Automatically reset mock state before every test. ' + 'Equivalent to calling jest.resetAllMocks() before each test.',
    type: 'boolean'
  },
  resetModules: {
    description: 'If enabled, the module registry for every test file will ' + 'be reset before running each individual test.',
    type: 'boolean'
  },
  resolver: {
    description: 'A JSON string which allows the use of a custom resolver.',
    requiresArg: true,
    type: 'string'
  },
  restoreMocks: {
    description: 'Automatically restore mock state and implementation before every test. ' + 'Equivalent to calling jest.restoreAllMocks() before each test.',
    type: 'boolean'
  },
  rootDir: {
    description: 'The root directory that Jest should scan for tests and ' + 'modules within.',
    requiresArg: true,
    type: 'string'
  },
  roots: {
    description: 'A list of paths to directories that Jest should use to ' + 'search for files in.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  runInBand: {
    alias: 'i',
    description: 'Run all tests serially in the current process (rather than ' + 'creating a worker pool of child processes that run tests). This ' + 'is sometimes useful for debugging, but such use cases are pretty ' + 'rare.',
    type: 'boolean'
  },
  runTestsByPath: {
    description: 'Used when provided patterns are exact file paths. This avoids ' + 'converting them into a regular expression and matching it against ' + 'every single file.',
    type: 'boolean'
  },
  runner: {
    description: "Allows to use a custom runner instead of Jest's default test runner.",
    requiresArg: true,
    type: 'string'
  },
  seed: {
    description: 'Sets a seed value that can be retrieved in a tests file via `jest.getSeed()`. If this option is not specified Jest will randomly generate the value. The seed value must be between `-0x80000000` and `0x7fffffff` inclusive.',
    requiresArg: true,
    type: 'number'
  },
  selectProjects: {
    description: 'Run the tests of the specified projects. ' + 'Jest uses the attribute `displayName` in the configuration to identify each project.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  setupFiles: {
    description: 'A list of paths to modules that run some code to configure or ' + 'set up the testing environment before each test.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  setupFilesAfterEnv: {
    description: 'A list of paths to modules that run some code to configure or ' + 'set up the testing framework before each test',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  shard: {
    description: 'Shard tests and execute only the selected shard, specify in ' + 'the form "current/all". 1-based, for example "3/5".',
    requiresArg: true,
    type: 'string'
  },
  showConfig: {
    description: 'Print your jest config and then exits.',
    type: 'boolean'
  },
  showSeed: {
    description: 'Prints the seed value in the test report summary. See `--seed` for how to set this value',
    type: 'boolean'
  },
  silent: {
    description: 'Prevent tests from printing messages through the console.',
    type: 'boolean'
  },
  skipFilter: {
    description: 'Disables the filter provided by --filter. Useful for CI jobs, or ' + 'local enforcement when fixing tests.',
    type: 'boolean'
  },
  snapshotSerializers: {
    description: 'A list of paths to snapshot serializer modules Jest should ' + 'use for snapshot testing.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  testEnvironment: {
    alias: 'env',
    description: 'The test environment used for all tests. This can point to ' + 'any file or node module. Examples: `jsdom`, `node` or ' + '`path/to/my-environment.js`',
    requiresArg: true,
    type: 'string'
  },
  testEnvironmentOptions: {
    description: 'A JSON string with options that will be passed to the `testEnvironment`. ' + 'The relevant options depend on the environment.',
    requiresArg: true,
    type: 'string'
  },
  testFailureExitCode: {
    description: 'Exit code of `jest` command if the test run failed',
    requiresArg: true,
    type: 'string' // number
  },
  testLocationInResults: {
    description: 'Add `location` information to the test results',
    type: 'boolean'
  },
  testMatch: {
    description: 'The glob patterns Jest uses to detect test files.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  testNamePattern: {
    alias: 't',
    description: 'Run only tests with a name that matches the regex pattern.',
    requiresArg: true,
    type: 'string'
  },
  testPathIgnorePatterns: {
    description: 'An array of regexp pattern strings that are matched ' + 'against all test paths before executing the test. If the test path ' + 'matches any of the patterns, it will be skipped.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  testPathPatterns: {
    description: 'An array of regexp pattern strings that are matched against all tests ' + 'paths before executing the test.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  testRegex: {
    description: 'A string or array of string regexp patterns that Jest uses to detect test files.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  testResultsProcessor: {
    description: 'Allows the use of a custom results processor. ' + 'This processor must be a node module that exports ' + 'a function expecting as the first argument the result object.',
    requiresArg: true,
    type: 'string'
  },
  testRunner: {
    description: 'Allows to specify a custom test runner. The default is' + ' `jest-circus/runner`. A path to a custom test runner can be provided:' + ' `<rootDir>/path/to/testRunner.js`.',
    requiresArg: true,
    type: 'string'
  },
  testSequencer: {
    description: 'Allows to specify a custom test sequencer. The default is ' + '`@jest/test-sequencer`. A path to a custom test sequencer can be ' + 'provided: `<rootDir>/path/to/testSequencer.js`',
    requiresArg: true,
    type: 'string'
  },
  testTimeout: {
    description: 'This option sets the default timeouts of test cases.',
    requiresArg: true,
    type: 'number'
  },
  transform: {
    description: 'A JSON string which maps from regular expressions to paths ' + 'to transformers.',
    requiresArg: true,
    type: 'string'
  },
  transformIgnorePatterns: {
    description: 'An array of regexp pattern strings that are matched ' + 'against all source file paths before transformation.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  unmockedModulePathPatterns: {
    description: 'An array of regexp pattern strings that are matched ' + 'against all modules before the module loader will automatically ' + 'return a mock for them.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  updateSnapshot: {
    alias: 'u',
    description: 'Use this flag to re-record snapshots. ' + 'Can be used together with a test suite pattern or with ' + '`--testNamePattern` to re-record snapshot for test matching ' + 'the pattern',
    type: 'boolean'
  },
  useStderr: {
    description: 'Divert all output to stderr.',
    type: 'boolean'
  },
  verbose: {
    description: 'Display individual test results with the test suite hierarchy.',
    type: 'boolean'
  },
  waitForUnhandledRejections: {
    description: 'Gives one event loop turn to handle `rejectionHandled`, ' + '`uncaughtException` or `unhandledRejection`.',
    type: 'boolean'
  },
  watch: {
    description: 'Watch files for changes and rerun tests related to ' + 'changed files. If you want to re-run all tests when a file has ' + 'changed, use the `--watchAll` option.',
    type: 'boolean'
  },
  watchAll: {
    description: 'Watch files for changes and rerun all tests. If you want ' + 'to re-run only the tests related to the changed files, use the ' + '`--watch` option.',
    type: 'boolean'
  },
  watchPathIgnorePatterns: {
    description: 'An array of regexp pattern strings that are matched ' + 'against all paths before trigger test re-run in watch mode. ' + 'If the test path matches any of the patterns, it will be skipped.',
    requiresArg: true,
    string: true,
    type: 'array'
  },
  watchman: {
    description: 'Whether to use watchman for file crawling. Disable using ' + '--no-watchman.',
    type: 'boolean'
  },
  workerGracefulExitTimeout: {
    description: 'Timeout in milliseconds for worker processes to exit gracefully ' + 'after tests complete. Workers that do not exit in time are ' + 'force-killed. Default: 500.',
    requiresArg: true,
    type: 'number'
  },
  workerThreads: {
    description: 'Whether to use worker threads for parallelization. Child processes ' + 'are used by default.',
    type: 'boolean'
  }
};

/***/ },

/***/ "./src/run.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.buildArgv = buildArgv;
exports.run = run;
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
function _exitX() {
  const data = _interopRequireDefault(require("exit-x"));
  _exitX = function () {
    return data;
  };
  return data;
}
function _yargs() {
  const data = _interopRequireDefault(require("yargs"));
  _yargs = function () {
    return data;
  };
  return data;
}
function _core() {
  const data = require("@jest/core");
  _core = function () {
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
var args = _interopRequireWildcard(__webpack_require__("./src/args.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

async function run(maybeArgv, project) {
  try {
    const argv = await buildArgv(maybeArgv);
    const projects = getProjectListFromCLIArgs(argv, project);
    const {
      results,
      globalConfig
    } = await (0, _core().runCLI)(argv, projects);
    readResultsAndExit(results, globalConfig);
  } catch (error) {
    (0, _jestUtil().clearLine)(process.stderr);
    (0, _jestUtil().clearLine)(process.stdout);
    if (error?.stack) {
      console.error(_chalk().default.red(error.stack));
    } else {
      console.error(_chalk().default.red(error));
    }
    (0, _exitX().default)(1);
    throw error;
  }
}
async function buildArgv(maybeArgv) {
  const version = (0, _core().getVersion)() + (__dirname.includes(`packages${path().sep}jest-cli`) ? '-dev' : '');
  const rawArgv = maybeArgv || process.argv.slice(2);
  const argv = await (0, _yargs().default)(rawArgv).usage(args.usage).version(version).alias('help', 'h').options(args.options).epilogue(args.docs).check(args.check).argv;
  (0, _jestValidate().validateCLIOptions)(argv, {
    ...args.options,
    deprecationEntries: _jestConfig().deprecationEntries
  },
  // strip leading dashes
  Array.isArray(rawArgv) ? rawArgv.map(rawArgv => rawArgv.replace(/^--?/, '')) : Object.keys(rawArgv));

  // strip dashed args
  return Object.keys(argv).reduce((result, key) => {
    if (!key.includes('-')) {
      result[key] = argv[key];
    }
    return result;
  }, {
    $0: argv.$0,
    _: argv._
  });
}
const getProjectListFromCLIArgs = (argv, project) => {
  const projects = argv.projects ?? [];
  if (project) {
    projects.push(project);
  }
  if (projects.length === 0 && process.platform === 'win32') {
    try {
      projects.push((0, _jestUtil().tryRealpath)(process.cwd()));
    } catch {
      // do nothing, just catch error
      // process.binding('fs').realpath can throw, e.g. on mapped drives
    }
  }
  if (projects.length === 0) {
    projects.push(process.cwd());
  }
  return projects;
};
const readResultsAndExit = (result, globalConfig) => {
  const code = !result || result.success ? 0 : globalConfig.testFailureExitCode;

  // Only exit if needed
  process.on('exit', () => {
    if (typeof code === 'number' && code !== 0) {
      process.exitCode = code;
    }
  });
  if (globalConfig.forceExit) {
    if (!globalConfig.detectOpenHandles) {
      console.warn(`${_chalk().default.bold('Force exiting Jest: ')}Have you considered using \`--detectOpenHandles\` to detect ` + 'async operations that kept running after all tests finished?');
    }
    (0, _exitX().default)(code);
  } else if (!globalConfig.detectOpenHandles && globalConfig.openHandlesTimeout !== 0) {
    const timeout = globalConfig.openHandlesTimeout;
    setTimeout(() => {
      console.warn(_chalk().default.yellow.bold(`Jest did not exit ${timeout === 1000 ? 'one second' : `${timeout / 1000} seconds`} after the test run has completed.\n\n'`) + _chalk().default.yellow('This usually means that there are asynchronous operations that ' + "weren't stopped in your tests. Consider running Jest with " + '`--detectOpenHandles` to troubleshoot this issue.'));
    }, timeout).unref();
  }
};

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
Object.defineProperty(exports, "buildArgv", ({
  enumerable: true,
  get: function () {
    return _run.buildArgv;
  }
}));
Object.defineProperty(exports, "run", ({
  enumerable: true,
  get: function () {
    return _run.run;
  }
}));
Object.defineProperty(exports, "yargsOptions", ({
  enumerable: true,
  get: function () {
    return _args.options;
  }
}));
var _run = __webpack_require__("./src/run.ts");
var _args = __webpack_require__("./src/args.ts");
})();

module.exports = __webpack_exports__;
/******/ })()
;