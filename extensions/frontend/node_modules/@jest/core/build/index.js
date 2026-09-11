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

/***/ "./src/FailedTestsCache.ts"
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

class FailedTestsCache {
  _enabledTestsMap;
  filterTests(tests) {
    const enabledTestsMap = this._enabledTestsMap;
    if (!enabledTestsMap) {
      return tests;
    }
    return tests.filter(test => enabledTestsMap[test.path]);
  }
  setTestResults(testResults) {
    this._enabledTestsMap = (testResults || []).reduce((suiteMap, testResult) => {
      if (testResult.testExecError) {
        suiteMap[testResult.testFilePath] = {};
        return suiteMap;
      }
      if (!testResult.numFailingTests) {
        return suiteMap;
      }
      suiteMap[testResult.testFilePath] = testResult.testResults.reduce((testMap, test) => {
        if (test.status !== 'failed') {
          return testMap;
        }
        testMap[test.fullName] = true;
        return testMap;
      }, {});
      return suiteMap;
    }, {});
    this._enabledTestsMap = Object.freeze(this._enabledTestsMap);
  }
}
exports["default"] = FailedTestsCache;

/***/ },

/***/ "./src/FailedTestsInteractiveMode.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _ansiEscapes() {
  const data = _interopRequireDefault(require("ansi-escapes"));
  _ansiEscapes = function () {
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
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

const {
  ARROW,
  CLEAR
} = _jestUtil().specialChars;
function describeKey(key, description) {
  return `${_chalk().default.dim(`${ARROW}Press`)} ${key} ${_chalk().default.dim(description)}`;
}
const TestProgressLabel = _chalk().default.bold('Interactive Test Progress');
class FailedTestsInteractiveMode {
  _isActive = false;
  _countPaths = 0;
  _skippedNum = 0;
  _testAssertions = [];
  _updateTestRunnerConfig;
  constructor(_pipe) {
    this._pipe = _pipe;
  }
  isActive() {
    return this._isActive;
  }
  put(key) {
    switch (key) {
      case 's':
        if (this._skippedNum === this._testAssertions.length) {
          break;
        }
        this._skippedNum += 1;
        // move skipped test to the end
        this._testAssertions.push(this._testAssertions.shift());
        if (this._testAssertions.length - this._skippedNum > 0) {
          this._run();
        } else {
          this._drawUIDoneWithSkipped();
        }
        break;
      case 'q':
      case _jestWatcher().KEYS.ESCAPE:
        this.abort();
        break;
      case 'r':
        this.restart();
        break;
      case _jestWatcher().KEYS.ENTER:
        if (this._testAssertions.length === 0) {
          this.abort();
        } else {
          this._run();
        }
        break;
      default:
    }
  }
  run(failedTestAssertions, updateConfig) {
    if (failedTestAssertions.length === 0) return;
    this._testAssertions = [...failedTestAssertions];
    this._countPaths = this._testAssertions.length;
    this._updateTestRunnerConfig = updateConfig;
    this._isActive = true;
    this._run();
  }
  updateWithResults(results) {
    if (!results.snapshot.failure && results.numFailedTests > 0) {
      return this._drawUIOverlay();
    }
    this._testAssertions.shift();
    if (this._testAssertions.length === 0) {
      return this._drawUIOverlay();
    }

    // Go to the next test
    return this._run();
  }
  _clearTestSummary() {
    this._pipe.write(_ansiEscapes().default.cursorUp(6));
    this._pipe.write(_ansiEscapes().default.eraseDown);
  }
  _drawUIDone() {
    this._pipe.write(CLEAR);
    const messages = [_chalk().default.bold('Watch Usage'), describeKey('Enter', 'to return to watch mode.')];
    this._pipe.write(`${messages.join('\n')}\n`);
  }
  _drawUIDoneWithSkipped() {
    this._pipe.write(CLEAR);
    let stats = `${(0, _jestUtil().pluralize)('test', this._countPaths)} reviewed`;
    if (this._skippedNum > 0) {
      const skippedText = _chalk().default.bold.yellow(`${(0, _jestUtil().pluralize)('test', this._skippedNum)} skipped`);
      stats = `${stats}, ${skippedText}`;
    }
    const message = [TestProgressLabel, `${ARROW}${stats}`, '\n', _chalk().default.bold('Watch Usage'), describeKey('r', 'to restart Interactive Mode.'), describeKey('q', 'to quit Interactive Mode.'), describeKey('Enter', 'to return to watch mode.')];
    this._pipe.write(`\n${message.join('\n')}`);
  }
  _drawUIProgress() {
    this._clearTestSummary();
    const numPass = this._countPaths - this._testAssertions.length;
    const numRemaining = this._countPaths - numPass - this._skippedNum;
    let stats = `${(0, _jestUtil().pluralize)('test', numRemaining)} remaining`;
    if (this._skippedNum > 0) {
      const skippedText = _chalk().default.bold.yellow(`${(0, _jestUtil().pluralize)('test', this._skippedNum)} skipped`);
      stats = `${stats}, ${skippedText}`;
    }
    const message = [TestProgressLabel, `${ARROW}${stats}`, '\n', _chalk().default.bold('Watch Usage'), describeKey('s', 'to skip the current test.'), describeKey('q', 'to quit Interactive Mode.'), describeKey('Enter', 'to return to watch mode.')];
    this._pipe.write(`\n${message.join('\n')}`);
  }
  _drawUIOverlay() {
    if (this._testAssertions.length === 0) return this._drawUIDone();
    return this._drawUIProgress();
  }
  _run() {
    if (this._updateTestRunnerConfig) {
      this._updateTestRunnerConfig(this._testAssertions[0]);
    }
  }
  abort() {
    this._isActive = false;
    this._skippedNum = 0;
    if (this._updateTestRunnerConfig) {
      this._updateTestRunnerConfig();
    }
  }
  restart() {
    this._skippedNum = 0;
    this._countPaths = this._testAssertions.length;
    this._run();
  }
}
exports["default"] = FailedTestsInteractiveMode;

/***/ },

/***/ "./src/ReporterDispatcher.ts"
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

class ReporterDispatcher {
  _reporters;
  constructor() {
    this._reporters = [];
  }
  register(reporter) {
    this._reporters.push(reporter);
  }
  unregister(reporterConstructor) {
    this._reporters = this._reporters.filter(reporter => !(reporter instanceof reporterConstructor));
  }
  async onTestFileResult(test, testResult, results) {
    for (const reporter of this._reporters) {
      if (reporter.onTestFileResult) {
        await reporter.onTestFileResult(test, testResult, results);
      } else if (reporter.onTestResult) {
        await reporter.onTestResult(test, testResult, results);
      }
    }

    // Release memory if unused later.
    testResult.coverage = undefined;
    testResult.console = undefined;
  }
  async onTestFileStart(test) {
    for (const reporter of this._reporters) {
      if (reporter.onTestFileStart) {
        await reporter.onTestFileStart(test);
      } else if (reporter.onTestStart) {
        await reporter.onTestStart(test);
      }
    }
  }
  async onRunStart(results, options) {
    for (const reporter of this._reporters) {
      if (reporter.onRunStart) {
        await reporter.onRunStart(results, options);
      }
    }
  }
  async onTestCaseStart(test, testCaseStartInfo) {
    for (const reporter of this._reporters) {
      if (reporter.onTestCaseStart) {
        await reporter.onTestCaseStart(test, testCaseStartInfo);
      }
    }
  }
  async onTestCaseResult(test, testCaseResult) {
    for (const reporter of this._reporters) {
      if (reporter.onTestCaseResult) {
        await reporter.onTestCaseResult(test, testCaseResult);
      }
    }
  }
  async onRunComplete(testContexts, results) {
    for (const reporter of this._reporters) {
      if (reporter.onRunComplete) {
        await reporter.onRunComplete(testContexts, results);
      }
    }
  }

  // Return a list of last errors for every reporter
  getErrors() {
    return this._reporters.reduce((list, reporter) => {
      const error = reporter.getLastError?.();
      return error ? [...list, error] : list;
    }, []);
  }
  hasErrors() {
    return this.getErrors().length > 0;
  }
}
exports["default"] = ReporterDispatcher;

/***/ },

/***/ "./src/SearchSource.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function os() {
  const data = _interopRequireWildcard(require("node:os"));
  os = function () {
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
function _jestConfig() {
  const data = require("jest-config");
  _jestConfig = function () {
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
function _jestResolveDependencies() {
  const data = require("jest-resolve-dependencies");
  _jestResolveDependencies = function () {
    return data;
  };
  return data;
}
function _jestSnapshot() {
  const data = require("jest-snapshot");
  _jestSnapshot = function () {
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

const regexToMatcher = testRegex => {
  const regexes = testRegex.map(testRegex => new RegExp(testRegex));
  return path => regexes.some(regex => {
    const result = regex.test(path);

    // prevent stateful regexes from breaking, just in case
    regex.lastIndex = 0;
    return result;
  });
};
const toTests = (context, tests) => tests.map(path => ({
  context,
  duration: undefined,
  path
}));
const hasSCM = changedFilesInfo => {
  const {
    repos
  } = changedFilesInfo;
  // no SCM (git/hg/...) is found in any of the roots.
  const noSCM = Object.values(repos).every(scm => scm.size === 0);
  return !noSCM;
};
function normalizePosix(filePath) {
  return filePath.replaceAll('\\', '/');
}
class SearchSource {
  _context;
  _dependencyResolver;
  _testPathCases = [];
  constructor(context) {
    const {
      config
    } = context;
    this._context = context;
    this._dependencyResolver = null;
    const rootPattern = new RegExp(config.roots.map(dir => (0, _jestRegexUtil().escapePathForRegex)(dir + path().sep)).join('|'));
    this._testPathCases.push({
      isMatch: path => rootPattern.test(path),
      stat: 'roots'
    });
    if (config.testMatch.length > 0) {
      this._testPathCases.push({
        isMatch: (0, _jestUtil().globsToMatcher)(config.testMatch),
        stat: 'testMatch'
      });
    }
    if (config.testPathIgnorePatterns.length > 0) {
      const testIgnorePatternsRegex = new RegExp(config.testPathIgnorePatterns.join('|'));
      this._testPathCases.push({
        isMatch: path => !testIgnorePatternsRegex.test(path),
        stat: 'testPathIgnorePatterns'
      });
    }
    if (config.testRegex.length > 0) {
      this._testPathCases.push({
        isMatch: regexToMatcher(config.testRegex),
        stat: 'testRegex'
      });
    }
  }
  async _getOrBuildDependencyResolver() {
    if (!this._dependencyResolver) {
      this._dependencyResolver = new (_jestResolveDependencies().DependencyResolver)(this._context.resolver, this._context.hasteFS, await (0, _jestSnapshot().buildSnapshotResolver)(this._context.config));
    }
    return this._dependencyResolver;
  }
  _filterTestPathsWithStats(allPaths, testPathPatternsExecutor) {
    const data = {
      stats: {
        roots: 0,
        testMatch: 0,
        testPathIgnorePatterns: 0,
        testRegex: 0
      },
      tests: [],
      total: allPaths.length
    };
    const testCases = [...this._testPathCases]; // clone
    if (testPathPatternsExecutor.isSet()) {
      testCases.push({
        isMatch: path => testPathPatternsExecutor.isMatch(path),
        stat: 'testPathPatterns'
      });
      data.stats.testPathPatterns = 0;
    }
    data.tests = allPaths.filter(test => {
      let filterResult = true;
      for (const {
        isMatch,
        stat
      } of testCases) {
        if (isMatch(test.path)) {
          data.stats[stat]++;
        } else {
          filterResult = false;
        }
      }
      return filterResult;
    });
    return data;
  }
  _getAllTestPaths(testPathPatternsExecutor) {
    return this._filterTestPathsWithStats(toTests(this._context, this._context.hasteFS.getAllFiles()), testPathPatternsExecutor);
  }
  isTestFilePath(path) {
    return this._testPathCases.every(testCase => testCase.isMatch(path));
  }
  findMatchingTests(testPathPatternsExecutor) {
    return this._getAllTestPaths(testPathPatternsExecutor);
  }
  async findRelatedTests(allPaths, collectCoverage) {
    const dependencyResolver = await this._getOrBuildDependencyResolver();
    if (!collectCoverage) {
      return {
        tests: toTests(this._context, dependencyResolver.resolveInverse(allPaths, this.isTestFilePath.bind(this), {
          skipNodeResolution: this._context.config.skipNodeResolution
        }))
      };
    }
    const testModulesMap = dependencyResolver.resolveInverseModuleMap(allPaths, this.isTestFilePath.bind(this), {
      skipNodeResolution: this._context.config.skipNodeResolution
    });
    const allPathsAbsolute = new Set([...allPaths].map(p => path().resolve(p)));
    const collectCoverageFrom = new Set();
    for (const testModule of testModulesMap) {
      if (!testModule.dependencies) {
        continue;
      }
      for (const p of testModule.dependencies) {
        if (!allPathsAbsolute.has(p)) {
          continue;
        }
        const filename = (0, _jestConfig().replaceRootDirInPath)(this._context.config.rootDir, p);
        collectCoverageFrom.add(path().isAbsolute(filename) ? path().relative(this._context.config.rootDir, filename) : filename);
      }
    }
    return {
      collectCoverageFrom,
      tests: toTests(this._context, testModulesMap.map(testModule => testModule.file))
    };
  }
  findTestsByPaths(paths) {
    return {
      tests: toTests(this._context, paths.map(p => path().resolve(this._context.config.cwd, p)).filter(this.isTestFilePath.bind(this)))
    };
  }
  async findRelatedTestsFromPattern(paths, collectCoverage) {
    if (Array.isArray(paths) && paths.length > 0) {
      const resolvedPaths = paths.map(p => path().resolve(this._context.config.cwd, p));
      return this.findRelatedTests(new Set(resolvedPaths), collectCoverage);
    }
    return {
      tests: []
    };
  }
  async findTestRelatedToChangedFiles(changedFilesInfo, collectCoverage) {
    if (!hasSCM(changedFilesInfo)) {
      return {
        noSCM: true,
        tests: []
      };
    }
    const {
      changedFiles
    } = changedFilesInfo;
    return this.findRelatedTests(changedFiles, collectCoverage);
  }
  async _getTestPaths(globalConfig, projectConfig, changedFiles) {
    if (globalConfig.onlyChanged) {
      if (!changedFiles) {
        throw new Error('Changed files must be set when running with -o.');
      }
      return this.findTestRelatedToChangedFiles(changedFiles, globalConfig.collectCoverage);
    }
    let paths = globalConfig.nonFlagArgs;
    if (globalConfig.findRelatedTests && 'win32' === os().platform()) {
      paths = this.filterPathsWin32(paths);
    }
    if (globalConfig.runTestsByPath && paths && paths.length > 0) {
      return this.findTestsByPaths(paths);
    } else if (globalConfig.findRelatedTests && paths && paths.length > 0) {
      return this.findRelatedTestsFromPattern(paths, globalConfig.collectCoverage);
    } else {
      return this.findMatchingTests(globalConfig.testPathPatterns.toExecutor({
        rootDir: projectConfig.rootDir
      }));
    }
  }
  filterPathsWin32(paths) {
    const allFiles = this._context.hasteFS.getAllFiles();
    const options = {
      nocase: true,
      windows: false
    };
    paths = paths.map(p => {
      // micromatch works with forward slashes: https://github.com/micromatch/micromatch#backslashes
      const normalizedPath = normalizePosix(path().resolve(this._context.config.cwd, p));
      const matcher = (0, _jestUtil().globsToMatcher)([normalizedPath], options);
      return allFiles.map(normalizePosix).find(matcher);
    }).filter(p => p !== undefined).map(p => path().resolve(p));
    return paths;
  }
  async getTestPaths(globalConfig, projectConfig, changedFiles, filter) {
    const searchResult = await this._getTestPaths(globalConfig, projectConfig, changedFiles);
    const filterPath = globalConfig.filter;
    if (filter) {
      const tests = searchResult.tests;
      const filterResult = await filter(tests.map(test => test.path));
      if (!Array.isArray(filterResult.filtered)) {
        throw new TypeError(`Filter ${filterPath} did not return a valid test list`);
      }
      const filteredSet = new Set(filterResult.filtered);
      return {
        ...searchResult,
        tests: tests.filter(test => filteredSet.has(test.path))
      };
    }
    return searchResult;
  }
  async findRelatedSourcesFromTestsInChangedFiles(changedFilesInfo) {
    if (!hasSCM(changedFilesInfo)) {
      return [];
    }
    const {
      changedFiles
    } = changedFilesInfo;
    const dependencyResolver = await this._getOrBuildDependencyResolver();
    const relatedSourcesSet = new Set();
    for (const filePath of changedFiles) {
      if (this.isTestFilePath(filePath)) {
        const sourcePaths = dependencyResolver.resolve(filePath, {
          skipNodeResolution: this._context.config.skipNodeResolution
        });
        for (const sourcePath of sourcePaths) relatedSourcesSet.add(sourcePath);
      }
    }
    return [...relatedSourcesSet];
  }
}
exports["default"] = SearchSource;

/***/ },

/***/ "./src/SnapshotInteractiveMode.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _ansiEscapes() {
  const data = _interopRequireDefault(require("ansi-escapes"));
  _ansiEscapes = function () {
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
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

const {
  ARROW,
  CLEAR
} = _jestUtil().specialChars;
class SnapshotInteractiveMode {
  _pipe;
  _isActive;
  _updateTestRunnerConfig;
  _testAssertions;
  _countPaths;
  _skippedNum;
  constructor(pipe) {
    this._pipe = pipe;
    this._isActive = false;
    this._skippedNum = 0;
  }
  isActive() {
    return this._isActive;
  }
  getSkippedNum() {
    return this._skippedNum;
  }
  _clearTestSummary() {
    this._pipe.write(_ansiEscapes().default.cursorUp(6));
    this._pipe.write(_ansiEscapes().default.eraseDown);
  }
  _drawUIProgress() {
    this._clearTestSummary();
    const numPass = this._countPaths - this._testAssertions.length;
    const numRemaining = this._countPaths - numPass - this._skippedNum;
    let stats = _chalk().default.bold.dim(`${(0, _jestUtil().pluralize)('snapshot', numRemaining)} remaining`);
    if (numPass) {
      stats += `, ${_chalk().default.bold.green(`${(0, _jestUtil().pluralize)('snapshot', numPass)} updated`)}`;
    }
    if (this._skippedNum) {
      stats += `, ${_chalk().default.bold.yellow(`${(0, _jestUtil().pluralize)('snapshot', this._skippedNum)} skipped`)}`;
    }
    const messages = [`\n${_chalk().default.bold('Interactive Snapshot Progress')}`, ARROW + stats, `\n${_chalk().default.bold('Watch Usage')}`, `${_chalk().default.dim(`${ARROW}Press `)}u${_chalk().default.dim(' to update failing snapshots for this test.')}`, `${_chalk().default.dim(`${ARROW}Press `)}s${_chalk().default.dim(' to skip the current test.')}`, `${_chalk().default.dim(`${ARROW}Press `)}q${_chalk().default.dim(' to quit Interactive Snapshot Mode.')}`, `${_chalk().default.dim(`${ARROW}Press `)}Enter${_chalk().default.dim(' to trigger a test run.')}`];
    this._pipe.write(`${messages.filter(Boolean).join('\n')}\n`);
  }
  _drawUIDoneWithSkipped() {
    this._pipe.write(CLEAR);
    const numPass = this._countPaths - this._testAssertions.length;
    let stats = _chalk().default.bold.dim(`${(0, _jestUtil().pluralize)('snapshot', this._countPaths)} reviewed`);
    if (numPass) {
      stats += `, ${_chalk().default.bold.green(`${(0, _jestUtil().pluralize)('snapshot', numPass)} updated`)}`;
    }
    if (this._skippedNum) {
      stats += `, ${_chalk().default.bold.yellow(`${(0, _jestUtil().pluralize)('snapshot', this._skippedNum)} skipped`)}`;
    }
    const messages = [`\n${_chalk().default.bold('Interactive Snapshot Result')}`, ARROW + stats, `\n${_chalk().default.bold('Watch Usage')}`, `${_chalk().default.dim(`${ARROW}Press `)}r${_chalk().default.dim(' to restart Interactive Snapshot Mode.')}`, `${_chalk().default.dim(`${ARROW}Press `)}q${_chalk().default.dim(' to quit Interactive Snapshot Mode.')}`];
    this._pipe.write(`${messages.filter(Boolean).join('\n')}\n`);
  }
  _drawUIDone() {
    this._pipe.write(CLEAR);
    const numPass = this._countPaths - this._testAssertions.length;
    let stats = _chalk().default.bold.dim(`${(0, _jestUtil().pluralize)('snapshot', this._countPaths)} reviewed`);
    if (numPass) {
      stats += `, ${_chalk().default.bold.green(`${(0, _jestUtil().pluralize)('snapshot', numPass)} updated`)}`;
    }
    const messages = [`\n${_chalk().default.bold('Interactive Snapshot Result')}`, ARROW + stats, `\n${_chalk().default.bold('Watch Usage')}`, `${_chalk().default.dim(`${ARROW}Press `)}Enter${_chalk().default.dim(' to return to watch mode.')}`];
    this._pipe.write(`${messages.filter(Boolean).join('\n')}\n`);
  }
  _drawUIOverlay() {
    if (this._testAssertions.length === 0) {
      return this._drawUIDone();
    }
    if (this._testAssertions.length - this._skippedNum === 0) {
      return this._drawUIDoneWithSkipped();
    }
    return this._drawUIProgress();
  }
  put(key) {
    switch (key) {
      case 's':
        if (this._skippedNum === this._testAssertions.length) break;
        this._skippedNum += 1;

        // move skipped test to the end
        this._testAssertions.push(this._testAssertions.shift());
        if (this._testAssertions.length - this._skippedNum > 0) {
          this._run(false);
        } else {
          this._drawUIDoneWithSkipped();
        }
        break;
      case 'u':
        this._run(true);
        break;
      case 'q':
      case _jestWatcher().KEYS.ESCAPE:
        this.abort();
        break;
      case 'r':
        this.restart();
        break;
      case _jestWatcher().KEYS.ENTER:
        if (this._testAssertions.length === 0) {
          this.abort();
        } else {
          this._run(false);
        }
        break;
      default:
        break;
    }
  }
  abort() {
    this._isActive = false;
    this._skippedNum = 0;
    this._updateTestRunnerConfig(null, false);
  }
  restart() {
    this._skippedNum = 0;
    this._countPaths = this._testAssertions.length;
    this._run(false);
  }
  updateWithResults(results) {
    const hasSnapshotFailure = !!results.snapshot.failure;
    if (hasSnapshotFailure) {
      this._drawUIOverlay();
      return;
    }
    this._testAssertions.shift();
    if (this._testAssertions.length - this._skippedNum === 0) {
      this._drawUIOverlay();
      return;
    }

    // Go to the next test
    this._run(false);
  }
  _run(shouldUpdateSnapshot) {
    const testAssertion = this._testAssertions[0];
    this._updateTestRunnerConfig(testAssertion, shouldUpdateSnapshot);
  }
  run(failedSnapshotTestAssertions, onConfigChange) {
    if (failedSnapshotTestAssertions.length === 0) {
      return;
    }
    this._testAssertions = [...failedSnapshotTestAssertions];
    this._countPaths = this._testAssertions.length;
    this._updateTestRunnerConfig = onConfigChange;
    this._isActive = true;
    this._run(false);
  }
}
exports["default"] = SnapshotInteractiveMode;

/***/ },

/***/ "./src/TestNamePatternPrompt.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
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

class TestNamePatternPrompt extends _jestWatcher().PatternPrompt {
  constructor(pipe, prompt) {
    super(pipe, prompt, 'tests');
  }
  _onChange(pattern, options) {
    super._onChange(pattern, options);
    this._printPrompt(pattern);
  }
  _printPrompt(pattern) {
    const pipe = this._pipe;
    (0, _jestWatcher().printPatternCaret)(pattern, pipe);
    (0, _jestWatcher().printRestoredPatternCaret)(pattern, this._currentUsageRows, pipe);
  }
}
exports["default"] = TestNamePatternPrompt;

/***/ },

/***/ "./src/TestPathPatternPrompt.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
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

class TestPathPatternPrompt extends _jestWatcher().PatternPrompt {
  constructor(pipe, prompt) {
    super(pipe, prompt, 'filenames');
  }
  _onChange(pattern, options) {
    super._onChange(pattern, options);
    this._printPrompt(pattern);
  }
  _printPrompt(pattern) {
    const pipe = this._pipe;
    (0, _jestWatcher().printPatternCaret)(pattern, pipe);
    (0, _jestWatcher().printRestoredPatternCaret)(pattern, this._currentUsageRows, pipe);
  }
}
exports["default"] = TestPathPatternPrompt;

/***/ },

/***/ "./src/TestScheduler.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.createTestScheduler = createTestScheduler;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
function _exitX() {
  const data = _interopRequireDefault(require("exit-x"));
  _exitX = function () {
    return data;
  };
  return data;
}
function _fastJsonStableStringify() {
  const data = _interopRequireDefault(require("fast-json-stable-stringify"));
  _fastJsonStableStringify = function () {
    return data;
  };
  return data;
}
function _reporters() {
  const data = require("@jest/reporters");
  _reporters = function () {
    return data;
  };
  return data;
}
function _testResult() {
  const data = require("@jest/test-result");
  _testResult = function () {
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
function _jestMessageUtil() {
  const data = require("jest-message-util");
  _jestMessageUtil = function () {
    return data;
  };
  return data;
}
function _jestSnapshot() {
  const data = require("jest-snapshot");
  _jestSnapshot = function () {
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
var _ReporterDispatcher = _interopRequireDefault(__webpack_require__("./src/ReporterDispatcher.ts"));
var _runGlobalHook = _interopRequireDefault(__webpack_require__("./src/runGlobalHook.ts"));
var _testSchedulerHelper = __webpack_require__("./src/testSchedulerHelper.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Env vars that indicate the process is running inside an AI coding agent.
// Based on the detection logic from the std-env package.
const AGENT_ENV_VARS = ['AI_AGENT', 'AUGMENT_AGENT', 'CLAUDE_CODE', 'CLAUDECODE', 'CODEX_SANDBOX', 'CODEX_THREAD_ID', 'CURSOR_AGENT', 'GEMINI_CLI', 'GOOSE_PROVIDER', 'OPENCODE', 'REPL_ID'];
function detectAgent() {
  return AGENT_ENV_VARS.some(key => key in process.env && process.env[key] !== '');
}
async function createTestScheduler(globalConfig, context) {
  return new TestScheduler(globalConfig, context);
}
class TestScheduler {
  _context;
  _dispatcher;
  _globalConfig;
  constructor(globalConfig, context) {
    this._context = context;
    this._dispatcher = new _ReporterDispatcher.default();
    this._globalConfig = globalConfig;
  }
  addReporter(reporter) {
    this._dispatcher.register(reporter);
  }
  removeReporter(reporterConstructor) {
    this._dispatcher.unregister(reporterConstructor);
  }
  async scheduleTests(tests, watcher) {
    await this._setupReporters(tests);
    const onTestFileStart = this._dispatcher.onTestFileStart.bind(this._dispatcher);
    const timings = [];
    const testContexts = new Set();
    for (const test of tests) {
      testContexts.add(test.context);
      if (test.duration) {
        timings.push(test.duration);
      }
    }
    const aggregatedResults = createAggregatedResults(tests.length);
    const estimatedTime = Math.ceil(getEstimatedTime(timings, this._globalConfig.maxWorkers) / 1000);
    const runInBand = (0, _testSchedulerHelper.shouldRunInBand)(tests, timings, this._globalConfig);
    const onResult = async (test, testResult) => {
      if (watcher.isInterrupted()) {
        return;
      }
      if (testResult.testResults.length === 0) {
        const message = 'Your test suite must contain at least one test.';
        return onFailure(test, {
          message,
          stack: new Error(message).stack
        });
      }

      // Throws when the context is leaked after executing a test.
      if (testResult.leaks) {
        const message = `${_chalk().default.red.bold('EXPERIMENTAL FEATURE!\n')}Your test suite is leaking memory. Please ensure all references are cleaned.\n` + '\n' + 'There is a number of things that can leak memory:\n' + '  - Async operations that have not finished (e.g. fs.readFile).\n' + '  - Timers not properly mocked (e.g. setInterval, setTimeout).\n' + '  - Keeping references to the global scope.';
        return onFailure(test, {
          message,
          stack: new Error(message).stack
        });
      }
      (0, _testResult().addResult)(aggregatedResults, testResult);
      await this._dispatcher.onTestFileResult(test, testResult, aggregatedResults);
      return this._bailIfNeeded(testContexts, aggregatedResults, watcher, tests);
    };
    const onFailure = async (test, error) => {
      if (watcher.isInterrupted()) {
        return;
      }
      const testResult = (0, _testResult().buildFailureTestResult)(test.path, error);
      testResult.failureMessage = (0, _jestMessageUtil().formatExecError)(testResult.testExecError, test.context.config, this._globalConfig, test.path);
      (0, _testResult().addResult)(aggregatedResults, testResult);
      await this._dispatcher.onTestFileResult(test, testResult, aggregatedResults);
    };
    const updateSnapshotState = async () => {
      const contextsWithSnapshotResolvers = await Promise.all([...testContexts].map(async context => [context, await (0, _jestSnapshot().buildSnapshotResolver)(context.config)]));
      for (const [context, snapshotResolver] of contextsWithSnapshotResolvers) {
        const status = (0, _jestSnapshot().cleanup)(context.hasteFS, this._globalConfig.updateSnapshot, snapshotResolver, context.config.testPathIgnorePatterns);
        aggregatedResults.snapshot.filesRemoved += status.filesRemoved;
        aggregatedResults.snapshot.filesRemovedList = [...(aggregatedResults.snapshot.filesRemovedList || []), ...status.filesRemovedList];
      }
      const updateAll = this._globalConfig.updateSnapshot === 'all';
      aggregatedResults.snapshot.didUpdate = updateAll;
      aggregatedResults.snapshot.failure = !!(!updateAll && (aggregatedResults.snapshot.unchecked || aggregatedResults.snapshot.unmatched || aggregatedResults.snapshot.filesRemoved));
    };
    await this._dispatcher.onRunStart(aggregatedResults, {
      estimatedTime,
      showStatus: !runInBand
    });
    const testRunners = Object.create(null);
    const contextsByTestRunner = new WeakMap();
    try {
      await Promise.all([...testContexts].map(async context => {
        const {
          config
        } = context;
        const runnerKey = `${config.runner}\0${stableRunnerOptionsKey(config.runnerOptions)}`;
        if (!testRunners[runnerKey]) {
          const transformer = await (0, _transform().createScriptTransformer)(config);
          const Runner = await transformer.requireAndTranspileModule(config.runner);
          const runner = new Runner(this._globalConfig, {
            changedFiles: this._context.changedFiles,
            sourcesRelatedToTestsInChangedFiles: this._context.sourcesRelatedToTestsInChangedFiles
          }, config.runnerOptions);
          testRunners[runnerKey] = runner;
          contextsByTestRunner.set(runner, context);
        }
      }));
      const testsByRunner = this._partitionTests(testRunners, tests);
      if (testsByRunner) {
        try {
          for (const runner of Object.keys(testRunners)) {
            const testRunner = testRunners[runner];
            const context = contextsByTestRunner.get(testRunner);
            (0, _jestUtil().invariant)(context);
            const tests = testsByRunner[runner];
            const testRunnerOptions = {
              serial: runInBand || Boolean(testRunner.isSerial)
            };
            if (testRunner.supportsEventEmitters) {
              const unsubscribes = [testRunner.on('test-file-start', ([test]) => onTestFileStart(test)), testRunner.on('test-file-success', ([test, testResult]) => onResult(test, testResult)), testRunner.on('test-file-failure', ([test, error]) => onFailure(test, error)), testRunner.on('test-case-start', ([testPath, testCaseStartInfo]) => {
                const test = {
                  context,
                  path: testPath
                };
                this._dispatcher.onTestCaseStart(test, testCaseStartInfo);
              }), testRunner.on('test-case-result', ([testPath, testCaseResult]) => {
                const test = {
                  context,
                  path: testPath
                };
                this._dispatcher.onTestCaseResult(test, testCaseResult);
              })];
              await testRunner.runTests(tests, watcher, testRunnerOptions);
              for (const sub of unsubscribes) sub();
            } else {
              await testRunner.runTests(tests, watcher, onTestFileStart, onResult, onFailure, testRunnerOptions);
            }
          }
        } catch (error) {
          if (!watcher.isInterrupted()) {
            throw error;
          }
        }
      }
    } catch (error) {
      aggregatedResults.runExecError = buildExecError(error);
      await this._dispatcher.onRunComplete(testContexts, aggregatedResults);
      throw error;
    }
    await updateSnapshotState();
    aggregatedResults.wasInterrupted = watcher.isInterrupted();
    await this._dispatcher.onRunComplete(testContexts, aggregatedResults);
    const anyTestFailures = !(aggregatedResults.numFailedTests === 0 && aggregatedResults.numRuntimeErrorTestSuites === 0);
    const anyReporterErrors = this._dispatcher.hasErrors();
    aggregatedResults.success = !(anyTestFailures || aggregatedResults.snapshot.failure || anyReporterErrors);
    return aggregatedResults;
  }
  _partitionTests(testRunners, tests) {
    if (Object.keys(testRunners).length > 1) {
      return tests.reduce((testRuns, test) => {
        const {
          config
        } = test.context;
        const runnerKey = `${config.runner}\0${stableRunnerOptionsKey(config.runnerOptions)}`;
        if (!testRuns[runnerKey]) {
          testRuns[runnerKey] = [];
        }
        testRuns[runnerKey].push(test);
        return testRuns;
      }, Object.create(null));
    } else if (tests.length > 0 && tests[0] != null) {
      // If there is only one runner, don't partition the tests.
      const {
        config
      } = tests[0].context;
      const runnerKey = `${config.runner}\0${stableRunnerOptionsKey(config.runnerOptions)}`;
      return Object.assign(Object.create(null), {
        [runnerKey]: tests
      });
    } else {
      return null;
    }
  }
  async _setupReporters(tests) {
    const {
      collectCoverage: coverage,
      notify
    } = this._globalConfig;
    const verbose = this._globalConfig.verbose || tests.some(t => t.context.config.verbose);
    const reporters = this._globalConfig.reporters || [[detectAgent() ? 'agent' : 'default', {}]];
    let summaryOptions = null;
    for (const [reporter, options] of reporters) {
      switch (reporter) {
        case 'agent':
          summaryOptions = options;
          this.addReporter(new (_reporters().AgentReporter)(this._globalConfig));
          break;
        case 'default':
          summaryOptions = options;
          this.addReporter(verbose ? new (_reporters().VerboseReporter)(this._globalConfig) : new (_reporters().DefaultReporter)(this._globalConfig));
          break;
        case 'github-actions':
          if (_ciInfo().GITHUB_ACTIONS) {
            this.addReporter(new (_reporters().GitHubActionsReporter)(this._globalConfig, options));
          }
          break;
        case 'summary':
          summaryOptions = options;
          break;
        default:
          await this._addCustomReporter(reporter, options);
      }
    }
    if (notify) {
      this.addReporter(new (_reporters().NotifyReporter)(this._globalConfig, this._context));
    }
    if (coverage) {
      this.addReporter(new (_reporters().CoverageReporter)(this._globalConfig, this._context));
    }
    if (summaryOptions != null) {
      this.addReporter(new (_reporters().SummaryReporter)(this._globalConfig, summaryOptions));
    }
  }
  async _addCustomReporter(reporter, options) {
    try {
      const Reporter = await (0, _jestUtil().requireOrImportModule)(reporter);
      this.addReporter(new Reporter(this._globalConfig, options, this._context));
    } catch (error) {
      error.message = `An error occurred while adding the reporter at path "${_chalk().default.bold(reporter)}".\n${error instanceof Error ? error.message : ''}`;
      throw error;
    }
  }
  async _bailIfNeeded(testContexts, aggregatedResults, watcher, allTests) {
    if (this._globalConfig.bail !== 0 && aggregatedResults.numFailedTests >= this._globalConfig.bail) {
      if (watcher.isWatchMode()) {
        await watcher.setState({
          interrupted: true
        });
        return;
      }
      try {
        await this._dispatcher.onRunComplete(testContexts, aggregatedResults);
      } finally {
        // Perform global teardown if client configures `bail`
        if (allTests.length > 0) {
          performance.mark('jest/globalTeardown:start');
          await (0, _runGlobalHook.default)({
            allTests,
            globalConfig: this._globalConfig,
            moduleName: 'globalTeardown'
          });
          performance.mark('jest/globalTeardown:end');
          (0, _exitX().default)(this._globalConfig.testFailureExitCode);
        }
      }
    }
  }
}
const createAggregatedResults = numTotalTestSuites => {
  const result = (0, _testResult().makeEmptyAggregatedTestResult)();
  result.numTotalTestSuites = numTotalTestSuites;
  result.startTime = Date.now();
  result.success = false;
  return result;
};
const getEstimatedTime = (timings, workers) => {
  if (timings.length === 0) {
    return 0;
  }
  const max = Math.max(...timings);
  return timings.length <= workers ? max : Math.max(timings.reduce((sum, time) => sum + time) / workers, max);
};
const strToError = errString => {
  const {
    message,
    stack
  } = (0, _jestMessageUtil().separateMessageFromStack)(errString);
  if (stack.length > 0) {
    return {
      message,
      stack
    };
  }
  const error = new (_jestUtil().ErrorWithStack)(message, buildExecError);
  return {
    message,
    stack: error.stack || ''
  };
};
const buildExecError = err => {
  if (typeof err === 'string' || err == null) {
    return strToError(err || 'Error');
  }
  const anyErr = err;
  if (typeof anyErr.message === 'string') {
    if (typeof anyErr.stack === 'string' && anyErr.stack.length > 0) {
      return anyErr;
    }
    return strToError(anyErr.message);
  }
  return strToError(JSON.stringify(err));
};
function stableRunnerOptionsKey(options) {
  if (options == null || Object.keys(options).length === 0) return '{}';
  return (0, _fastJsonStableStringify().default)(options);
}

/***/ },

/***/ "./src/cli/index.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.runCLI = runCLI;
function _nodePerf_hooks() {
  const data = require("node:perf_hooks");
  _nodePerf_hooks = function () {
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
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
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
function _jestConfig() {
  const data = require("jest-config");
  _jestConfig = function () {
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
    return data;
  };
  return data;
}
var _collectHandles = __webpack_require__("./src/collectHandles.ts");
var _getChangedFilesPromise = _interopRequireDefault(__webpack_require__("./src/getChangedFilesPromise.ts"));
var _getConfigsOfProjectsToRun = _interopRequireDefault(__webpack_require__("./src/getConfigsOfProjectsToRun.ts"));
var _getProjectNamesMissingWarning = _interopRequireDefault(__webpack_require__("./src/getProjectNamesMissingWarning.ts"));
var _getSelectProjectsMessage = _interopRequireDefault(__webpack_require__("./src/getSelectProjectsMessage.ts"));
var _createContext = _interopRequireDefault(__webpack_require__("./src/lib/createContext.ts"));
var _handleDeprecationWarnings = _interopRequireDefault(__webpack_require__("./src/lib/handleDeprecationWarnings.ts"));
var _logDebugMessages = _interopRequireDefault(__webpack_require__("./src/lib/logDebugMessages.ts"));
var _runJest = _interopRequireDefault(__webpack_require__("./src/runJest.ts"));
var _watch = _interopRequireDefault(__webpack_require__("./src/watch.ts"));
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const {
  print: preRunMessagePrint
} = _jestUtil().preRunMessage;
async function runCLI(argv, projects) {
  _nodePerf_hooks().performance.mark('jest/runCLI:start');
  let results;

  // If we output a JSON object, we can't write anything to stdout, since
  // it'll break the JSON structure and it won't be valid.
  const outputStream = argv.json || argv.useStderr ? process.stderr : process.stdout;
  const {
    globalConfig,
    configs,
    hasDeprecationWarnings
  } = await (0, _jestConfig().readConfigs)(argv, projects);
  if (argv.debug) {
    (0, _logDebugMessages.default)(globalConfig, configs, outputStream);
  }
  if (argv.showConfig) {
    (0, _logDebugMessages.default)(globalConfig, configs, process.stdout);
    (0, _exitX().default)(0);
  }
  if (argv.clearCache) {
    // stick in a Set to dedupe the deletions
    const uniqueConfigDirectories = new Set(configs.map(config => config.cacheDirectory));
    for (const cacheDirectory of uniqueConfigDirectories) {
      fs().rmSync(cacheDirectory, {
        force: true,
        recursive: true
      });
      process.stdout.write(`Cleared ${cacheDirectory}\n`);
    }
    (0, _exitX().default)(0);
  }
  const configsOfProjectsToRun = (0, _getConfigsOfProjectsToRun.default)(configs, {
    ignoreProjects: argv.ignoreProjects,
    selectProjects: argv.selectProjects
  });
  if (argv.selectProjects || argv.ignoreProjects) {
    const namesMissingWarning = (0, _getProjectNamesMissingWarning.default)(configs, {
      ignoreProjects: argv.ignoreProjects,
      selectProjects: argv.selectProjects
    });
    if (namesMissingWarning) {
      outputStream.write(namesMissingWarning);
    }
    outputStream.write((0, _getSelectProjectsMessage.default)(configsOfProjectsToRun, {
      ignoreProjects: argv.ignoreProjects,
      selectProjects: argv.selectProjects
    }));
  }
  await _run10000(globalConfig, configsOfProjectsToRun, hasDeprecationWarnings, outputStream, r => {
    results = r;
  });
  if (argv.watch || argv.watchAll) {
    // If in watch mode, return the promise that will never resolve.
    // If the watch mode is interrupted, watch should handle the process
    // shutdown.
    // eslint-disable-next-line @typescript-eslint/no-empty-function
    return new Promise(() => {});
  }
  if (!results) {
    throw new Error('AggregatedResult must be present after test run is complete');
  }
  const {
    openHandles
  } = results;
  if (openHandles && openHandles.length > 0) {
    const formatted = (0, _collectHandles.formatHandleErrors)(openHandles, configs[0]);
    const openHandlesString = (0, _jestUtil().pluralize)('open handle', formatted.length, 's');
    const message = _chalk().default.red(`\nJest has detected the following ${openHandlesString} potentially keeping Jest from exiting:\n\n`) + formatted.join('\n\n');
    console.error(message);
  }
  _nodePerf_hooks().performance.mark('jest/runCLI:end');
  return {
    globalConfig,
    results
  };
}
const buildContextsAndHasteMaps = async (configs, globalConfig, outputStream) => {
  const hasteMapInstances = Array.from({
    length: configs.length
  });
  const contexts = await Promise.all(configs.map(async (config, index) => {
    (0, _jestUtil().createDirectory)(config.cacheDirectory);
    const hasteMapInstance = await _jestRuntime().default.createHasteMap(config, {
      console: new (_console().CustomConsole)(outputStream, outputStream),
      maxWorkers: Math.max(1, Math.floor(globalConfig.maxWorkers / configs.length)),
      resetCache: !config.cache,
      watch: globalConfig.watch || globalConfig.watchAll,
      watchman: globalConfig.watchman,
      workerThreads: globalConfig.workerThreads
    });
    hasteMapInstances[index] = hasteMapInstance;
    return (0, _createContext.default)(config, await hasteMapInstance.build());
  }));
  return {
    contexts,
    hasteMapInstances
  };
};
const _run10000 = async (globalConfig, configs, hasDeprecationWarnings, outputStream, onComplete) => {
  // Queries to hg/git can take a while, so we need to start the process
  // as soon as possible, so by the time we need the result it's already there.
  const changedFilesPromise = (0, _getChangedFilesPromise.default)(globalConfig, configs);
  if (changedFilesPromise) {
    _nodePerf_hooks().performance.mark('jest/getChangedFiles:start');
    changedFilesPromise.finally(() => {
      _nodePerf_hooks().performance.mark('jest/getChangedFiles:end');
    });
  }

  // Filter may need to do an HTTP call or something similar to setup.
  // We will wait on an async response from this before using the filter.
  let filter;
  if (globalConfig.filter && !globalConfig.skipFilter) {
    const rawFilter = require(globalConfig.filter);
    let filterSetupPromise;
    if (rawFilter.setup) {
      // Wrap filter setup Promise to avoid "uncaught Promise" error.
      // If an error is returned, we surface it in the return value.
      filterSetupPromise = (async () => {
        try {
          await rawFilter.setup();
        } catch (error) {
          return error;
        }
        return undefined;
      })();
    }
    filter = async testPaths => {
      if (filterSetupPromise) {
        // Expect an undefined return value unless there was an error.
        const err = await filterSetupPromise;
        if (err) {
          throw err;
        }
      }
      return rawFilter(testPaths);
    };
  }
  _nodePerf_hooks().performance.mark('jest/buildContextsAndHasteMaps:start');
  const {
    contexts,
    hasteMapInstances
  } = await buildContextsAndHasteMaps(configs, globalConfig, outputStream);
  _nodePerf_hooks().performance.mark('jest/buildContextsAndHasteMaps:end');
  if (globalConfig.watch || globalConfig.watchAll) {
    await runWatch(contexts, configs, hasDeprecationWarnings, globalConfig, outputStream, hasteMapInstances, filter);
  } else {
    await runWithoutWatch(globalConfig, contexts, outputStream, onComplete, changedFilesPromise, filter);
  }
};
const runWatch = async (contexts, _configs, hasDeprecationWarnings, globalConfig, outputStream, hasteMapInstances, filter) => {
  if (hasDeprecationWarnings) {
    try {
      await (0, _handleDeprecationWarnings.default)(outputStream, process.stdin);
      return await (0, _watch.default)(globalConfig, contexts, outputStream, hasteMapInstances, undefined, undefined, filter);
    } catch {
      (0, _exitX().default)(0);
    }
  }
  return (0, _watch.default)(globalConfig, contexts, outputStream, hasteMapInstances, undefined, undefined, filter);
};
const runWithoutWatch = async (globalConfig, contexts, outputStream, onComplete, changedFilesPromise, filter) => {
  const startRun = async () => {
    if (!globalConfig.listTests) {
      preRunMessagePrint(outputStream);
    }
    return (0, _runJest.default)({
      changedFilesPromise,
      contexts,
      failedTestsCache: undefined,
      filter,
      globalConfig,
      onComplete,
      outputStream,
      startRun,
      testWatcher: new (_jestWatcher().TestWatcher)({
        isWatchMode: false
      })
    });
  };
  return startRun();
};

/***/ },

/***/ "./src/collectHandles.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = collectHandles;
exports.formatHandleErrors = formatHandleErrors;
function asyncHooks() {
  const data = _interopRequireWildcard(require("node:async_hooks"));
  asyncHooks = function () {
    return data;
  };
  return data;
}
function _nodeUtil() {
  const data = require("node:util");
  _nodeUtil = function () {
    return data;
  };
  return data;
}
function v8() {
  const data = _interopRequireWildcard(require("node:v8"));
  v8 = function () {
    return data;
  };
  return data;
}
function vm() {
  const data = _interopRequireWildcard(require("node:vm"));
  vm = function () {
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

function stackIsFromUser(stack) {
  // Either the test file, or something required by it
  if (stack.includes('Runtime.requireModule')) {
    return true;
  }

  // jest-jasmine it or describe call
  if (stack.includes('asyncJestTest') || stack.includes('asyncJestLifecycle')) {
    return true;
  }

  // An async function call from within circus
  if (stack.includes('callAsyncCircusFn')) {
    // jest-circus it or describe call
    return stack.includes('_callCircusTest') || stack.includes('_callCircusHook');
  }
  return false;
}
const alwaysActive = () => true;
const hasWeakRef = typeof WeakRef === 'function';
const asyncSleep = (0, _nodeUtil().promisify)(setTimeout);
let gcFunc = globalThis.gc;
function runGC() {
  if (!gcFunc) {
    v8().setFlagsFromString('--expose-gc');
    gcFunc = vm().runInNewContext('gc');
    v8().setFlagsFromString('--no-expose-gc');
    if (!gcFunc) {
      throw new Error('Cannot find `global.gc` function. Please run node with `--expose-gc` and report this issue in jest repo.');
    }
  }
  gcFunc();
}

// Inspired by https://github.com/mafintosh/why-is-node-running/blob/master/index.js
// Extracted as we want to format the result ourselves
function collectHandles() {
  const activeHandles = new Map();
  const hook = asyncHooks().createHook({
    destroy(asyncId) {
      activeHandles.delete(asyncId);
    },
    init: function initHook(asyncId, type, triggerAsyncId,
    // eslint-disable-next-line @typescript-eslint/no-empty-object-type
    resource) {
      // Skip resources that should not generally prevent the process from
      // exiting, not last a meaningfully long time, or otherwise shouldn't be
      // tracked.
      if (['PROMISE', 'TIMERWRAP', 'ELDHISTOGRAM', 'PerformanceObserver', 'RANDOMBYTESREQUEST', 'DNSCHANNEL', 'ZLIB', 'SIGNREQUEST', 'TLSWRAP', 'TCPWRAP'].includes(type)) {
        return;
      }
      const error = new (_jestUtil().ErrorWithStack)(type, initHook, 100);
      let fromUser = stackIsFromUser(error.stack || '');

      // If the async resource was not directly created by user code, but was
      // triggered by another async resource from user code, track it and use
      // the original triggering resource's stack.
      if (!fromUser) {
        const triggeringHandle = activeHandles.get(triggerAsyncId);
        if (triggeringHandle) {
          fromUser = true;
          error.stack = triggeringHandle.error.stack;
        }
      }
      if (fromUser) {
        let isActive;

        // Handle that supports hasRef
        if ('hasRef' in resource) {
          if (hasWeakRef) {
            const ref = new WeakRef(resource);
            isActive = () => {
              return ref.deref()?.hasRef() ?? false;
            };
          } else {
            isActive = resource.hasRef.bind(resource);
          }
        } else {
          // Handle that doesn't support hasRef
          isActive = alwaysActive;
        }
        activeHandles.set(asyncId, {
          error,
          isActive
        });
      }
    }
  });
  hook.enable();
  return async () => {
    // Wait briefly for any async resources that have been queued for
    // destruction to actually be destroyed.
    // For example, Node.js TCP Servers are not destroyed until *after* their
    // `close` callback runs. If someone finishes a test from the `close`
    // callback, we will not yet have seen the resource be destroyed here.
    await asyncSleep(0);
    if (activeHandles.size > 0) {
      await asyncSleep(30);
      if (activeHandles.size > 0) {
        runGC();
        await asyncSleep(0);
      }
    }
    hook.disable();

    // Get errors for every async resource still referenced at this moment
    const result = [...activeHandles.values()].filter(({
      isActive
    }) => isActive()).map(({
      error
    }) => error);
    activeHandles.clear();
    return result;
  };
}
function formatHandleErrors(errors, config) {
  const stacks = new Map();
  for (const err of errors) {
    const formatted = (0, _jestMessageUtil().formatExecError)(err, config, {
      noStackTrace: false
    }, undefined, true);

    // E.g. timeouts might give multiple traces to the same line of code
    // This hairy filtering tries to remove entries with duplicate stack traces

    const ansiFree = (0, _nodeUtil().stripVTControlCharacters)(formatted);
    const match = ansiFree.match(/\s+at(.*)/);
    if (!match || match.length < 2) {
      continue;
    }
    const stackText = ansiFree.slice(ansiFree.indexOf(match[1])).trim();
    const name = ansiFree.match(/(?<=● {2}).*$/m);
    if (name == null || name.length === 0) {
      continue;
    }
    const stack = stacks.get(stackText) || {
      names: new Set(),
      stack: formatted.replace(name[0], '%%OBJECT_NAME%%')
    };
    stack.names.add(name[0]);
    stacks.set(stackText, stack);
  }
  return [...stacks.values()].map(({
    stack,
    names
  }) => stack.replace('%%OBJECT_NAME%%', [...names].join(',')));
}

/***/ },

/***/ "./src/getChangedFilesPromise.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getChangedFilesPromise;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _jestChangedFiles() {
  const data = require("jest-changed-files");
  _jestChangedFiles = function () {
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
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getChangedFilesPromise(globalConfig, configs) {
  if (globalConfig.onlyChanged) {
    const allRootsForAllProjects = new Set(configs.flatMap(config => config.roots || []));
    return (0, _jestChangedFiles().getChangedFilesForRoots)([...allRootsForAllProjects], {
      changedSince: globalConfig.changedSince,
      lastCommit: globalConfig.lastCommit,
      withAncestor: globalConfig.changedFilesWithAncestor
    }).catch(error => {
      const message = (0, _jestMessageUtil().formatExecError)(error, configs[0], {
        noStackTrace: true
      }).split('\n').filter(line => !line.includes('Command failed:')).join('\n');
      console.error(_chalk().default.red(`\n\n${message}`));
      process.exit(1);
    });
  }
  return undefined;
}

/***/ },

/***/ "./src/getConfigsOfProjectsToRun.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getConfigsOfProjectsToRun;
var _getProjectDisplayName = _interopRequireDefault(__webpack_require__("./src/getProjectDisplayName.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getConfigsOfProjectsToRun(projectConfigs, opts) {
  const projectFilter = createProjectFilter(opts);
  return projectConfigs.filter(config => {
    const name = (0, _getProjectDisplayName.default)(config);
    return projectFilter(name);
  });
}
const always = () => true;
function createProjectFilter(opts) {
  const {
    selectProjects,
    ignoreProjects
  } = opts;
  const selected = selectProjects ? name => name && selectProjects.includes(name) : always;
  const notIgnore = ignoreProjects ? name => !(name && ignoreProjects.includes(name)) : always;
  function test(name) {
    return selected(name) && notIgnore(name);
  }
  return test;
}

/***/ },

/***/ "./src/getNoTestFound.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getNoTestFound;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getNoTestFound(testRunData, globalConfig, willExitWith0) {
  const testFiles = testRunData.reduce((current, testRun) => current + (testRun.matches.total || 0), 0);
  let dataMessage;
  if (globalConfig.runTestsByPath) {
    dataMessage = `Files: ${globalConfig.nonFlagArgs.map(p => `"${p}"`).join(', ')}`;
  } else {
    dataMessage = `Pattern: ${_chalk().default.yellow(globalConfig.testPathPatterns.toPretty())} - 0 matches`;
  }
  if (willExitWith0) {
    return `${_chalk().default.bold('No tests found, exiting with code 0')}\n` + `In ${_chalk().default.bold(globalConfig.rootDir)}` + '\n' + `  ${(0, _jestUtil().pluralize)('file', testFiles, 's')} checked across ${(0, _jestUtil().pluralize)('project', testRunData.length, 's')}. Run with \`--verbose\` for more details.` + `\n${dataMessage}`;
  }
  return `${_chalk().default.bold('No tests found, exiting with code 1')}\n` + 'Run with `--passWithNoTests` to exit with code 0' + '\n' + `In ${_chalk().default.bold(globalConfig.rootDir)}` + '\n' + `  ${(0, _jestUtil().pluralize)('file', testFiles, 's')} checked across ${(0, _jestUtil().pluralize)('project', testRunData.length, 's')}. Run with \`--verbose\` for more details.` + `\n${dataMessage}`;
}

/***/ },

/***/ "./src/getNoTestFoundFailed.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getNoTestFoundFailed;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getNoTestFoundFailed(globalConfig) {
  let msg = _chalk().default.bold('No failed test found.');
  if (_jestUtil().isInteractive) {
    msg += _chalk().default.dim(`\n${globalConfig.watch ? 'Press `f` to quit "only failed tests" mode.' : 'Run Jest without `--onlyFailures` or with `--all` to run all tests.'}`);
  }
  return msg;
}

/***/ },

/***/ "./src/getNoTestFoundPassWithNoTests.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getNoTestFoundPassWithNoTests;
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

function getNoTestFoundPassWithNoTests() {
  return _chalk().default.bold('No tests found, exiting with code 0');
}

/***/ },

/***/ "./src/getNoTestFoundRelatedToChangedFiles.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getNoTestFoundRelatedToChangedFiles;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getNoTestFoundRelatedToChangedFiles(globalConfig) {
  const ref = globalConfig.changedSince ? `"${globalConfig.changedSince}"` : 'last commit';
  let msg = _chalk().default.bold(`No tests found related to files changed since ${ref}.`);
  if (_jestUtil().isInteractive) {
    msg += _chalk().default.dim(`\n${globalConfig.watch ? 'Press `a` to run all tests, or run Jest with `--watchAll`.' : 'Run Jest without `-o` or with `--all` to run all tests.'}`);
  }
  return msg;
}

/***/ },

/***/ "./src/getNoTestFoundVerbose.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getNoTestFoundVerbose;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getNoTestFoundVerbose(testRunData, globalConfig, willExitWith0) {
  const individualResults = testRunData.map(testRun => {
    const stats = testRun.matches.stats || {};
    const config = testRun.context.config;
    const statsMessage = Object.keys(stats).map(key => {
      if (key === 'roots' && config.roots.length === 1) {
        return null;
      }
      const value = config[key];
      if (value) {
        const valueAsString = Array.isArray(value) ? value.join(', ') : String(value);
        const matches = (0, _jestUtil().pluralize)('match', stats[key] || 0, 'es');
        return `  ${key}: ${_chalk().default.yellow(valueAsString)} - ${matches}`;
      }
      return null;
    }).filter(Boolean).join('\n');
    return testRun.matches.total ? `In ${_chalk().default.bold(config.rootDir)}\n` + `  ${(0, _jestUtil().pluralize)('file', testRun.matches.total || 0, 's')} checked.\n${statsMessage}` : `No files found in ${config.rootDir}.\n` + "Make sure Jest's configuration does not exclude this directory." + '\nTo set up Jest, make sure a package.json file exists.\n' + 'Jest Documentation: ' + 'https://jestjs.io/docs/configuration';
  });
  let dataMessage;
  if (globalConfig.runTestsByPath) {
    dataMessage = `Files: ${globalConfig.nonFlagArgs.map(p => `"${p}"`).join(', ')}`;
  } else {
    dataMessage = `Pattern: ${_chalk().default.yellow(globalConfig.testPathPatterns.toPretty())} - 0 matches`;
  }
  if (willExitWith0) {
    return `${_chalk().default.bold('No tests found, exiting with code 0')}\n${individualResults.join('\n')}\n${dataMessage}`;
  }
  return `${_chalk().default.bold('No tests found, exiting with code 1')}\n` + 'Run with `--passWithNoTests` to exit with code 0' + `\n${individualResults.join('\n')}\n${dataMessage}`;
}

/***/ },

/***/ "./src/getNoTestsFoundMessage.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getNoTestsFoundMessage;
var _getNoTestFound = _interopRequireDefault(__webpack_require__("./src/getNoTestFound.ts"));
var _getNoTestFoundFailed = _interopRequireDefault(__webpack_require__("./src/getNoTestFoundFailed.ts"));
var _getNoTestFoundPassWithNoTests = _interopRequireDefault(__webpack_require__("./src/getNoTestFoundPassWithNoTests.ts"));
var _getNoTestFoundRelatedToChangedFiles = _interopRequireDefault(__webpack_require__("./src/getNoTestFoundRelatedToChangedFiles.ts"));
var _getNoTestFoundVerbose = _interopRequireDefault(__webpack_require__("./src/getNoTestFoundVerbose.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getNoTestsFoundMessage(testRunData, globalConfig) {
  const exitWith0 = globalConfig.passWithNoTests || globalConfig.lastCommit || globalConfig.onlyChanged;
  if (globalConfig.onlyFailures) {
    return {
      exitWith0,
      message: (0, _getNoTestFoundFailed.default)(globalConfig)
    };
  }
  if (globalConfig.onlyChanged) {
    return {
      exitWith0,
      message: (0, _getNoTestFoundRelatedToChangedFiles.default)(globalConfig)
    };
  }
  if (globalConfig.passWithNoTests) {
    return {
      exitWith0,
      message: (0, _getNoTestFoundPassWithNoTests.default)()
    };
  }
  return {
    exitWith0,
    message: testRunData.length === 1 || globalConfig.verbose ? (0, _getNoTestFoundVerbose.default)(testRunData, globalConfig, exitWith0) : (0, _getNoTestFound.default)(testRunData, globalConfig, exitWith0)
  };
}

/***/ },

/***/ "./src/getProjectDisplayName.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getProjectDisplayName;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getProjectDisplayName(projectConfig) {
  return projectConfig.displayName?.name || undefined;
}

/***/ },

/***/ "./src/getProjectNamesMissingWarning.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getProjectNamesMissingWarning;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
var _getProjectDisplayName = _interopRequireDefault(__webpack_require__("./src/getProjectDisplayName.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getProjectNamesMissingWarning(projectConfigs, opts) {
  const numberOfProjectsWithoutAName = projectConfigs.filter(config => !(0, _getProjectDisplayName.default)(config)).length;
  if (numberOfProjectsWithoutAName === 0) {
    return undefined;
  }
  const args = [];
  if (opts.selectProjects) {
    args.push('--selectProjects');
  }
  if (opts.ignoreProjects) {
    args.push('--ignoreProjects');
  }
  return _chalk().default.yellow(`You provided values for ${args.join(' and ')} but ${numberOfProjectsWithoutAName === 1 ? 'a project does not have a name' : `${numberOfProjectsWithoutAName} projects do not have a name`}.\n` + 'Set displayName in the config of all projects in order to disable this warning.\n');
}

/***/ },

/***/ "./src/getSelectProjectsMessage.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getSelectProjectsMessage;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
var _getProjectDisplayName = _interopRequireDefault(__webpack_require__("./src/getProjectDisplayName.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getSelectProjectsMessage(projectConfigs, opts) {
  if (projectConfigs.length === 0) {
    return getNoSelectionWarning(opts);
  }
  return getProjectsRunningMessage(projectConfigs);
}
function getNoSelectionWarning(opts) {
  if (opts.ignoreProjects && opts.selectProjects) {
    return _chalk().default.yellow('You provided values for --selectProjects and --ignoreProjects, but no projects were found matching the selection.\n' + 'Are you ignoring all the selected projects?\n');
  } else if (opts.ignoreProjects) {
    return _chalk().default.yellow('You provided values for --ignoreProjects, but no projects were found matching the selection.\n' + 'Are you ignoring all projects?\n');
  } else if (opts.selectProjects) {
    return _chalk().default.yellow('You provided values for --selectProjects but no projects were found matching the selection.\n');
  } else {
    return _chalk().default.yellow('No projects were found.\n');
  }
}
function getProjectsRunningMessage(projectConfigs) {
  if (projectConfigs.length === 1) {
    const name = (0, _getProjectDisplayName.default)(projectConfigs[0]) ?? '<unnamed project>';
    return `Running one project: ${_chalk().default.bold(name)}\n`;
  }
  const projectsList = projectConfigs.map(getProjectNameListElement).sort().join('\n');
  return `Running ${projectConfigs.length} projects:\n${projectsList}\n`;
}
function getProjectNameListElement(projectConfig) {
  const name = (0, _getProjectDisplayName.default)(projectConfig);
  const elementContent = name ? _chalk().default.bold(name) : '<unnamed project>';
  return `- ${elementContent}`;
}

/***/ },

/***/ "./src/lib/activeFiltersMessage.ts"
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
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

const activeFilters = globalConfig => {
  const {
    testNamePattern
  } = globalConfig;
  const testPathPatterns = globalConfig.testPathPatterns;
  if (testNamePattern || testPathPatterns.isSet()) {
    const filters = [testPathPatterns.isSet() ? _chalk().default.dim('filename ') + _chalk().default.yellow(testPathPatterns.toPretty()) : null, testNamePattern ? _chalk().default.dim('test name ') + _chalk().default.yellow(`/${testNamePattern}/`) : null].filter(_jestUtil().isNonNullable).join(', ');
    const messages = `\n${_chalk().default.bold('Active Filters: ')}${filters}`;
    return messages;
  }
  return '';
};
var _default = exports["default"] = activeFilters;

/***/ },

/***/ "./src/lib/createContext.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = createContext;
function _jestRuntime() {
  const data = _interopRequireDefault(require("jest-runtime"));
  _jestRuntime = function () {
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

function createContext(config, {
  hasteFS,
  moduleMap
}) {
  return {
    config,
    hasteFS,
    moduleMap,
    resolver: _jestRuntime().default.createResolver(config, moduleMap)
  };
}

/***/ },

/***/ "./src/lib/handleDeprecationWarnings.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = handleDeprecationWarnings;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
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

function handleDeprecationWarnings(pipe, stdin = process.stdin) {
  return new Promise((resolve, reject) => {
    if (typeof stdin.setRawMode === 'function') {
      const messages = [_chalk().default.red('There are deprecation warnings.\n'), `${_chalk().default.dim(' \u203A Press ')}Enter${_chalk().default.dim(' to continue.')}`, `${_chalk().default.dim(' \u203A Press ')}Esc${_chalk().default.dim(' to exit.')}`];
      pipe.write(messages.join('\n'));
      stdin.setRawMode(true);
      stdin.resume();
      stdin.setEncoding('utf8');
      // this is a string since we set encoding above
      stdin.on('data', key => {
        if (key === _jestWatcher().KEYS.ENTER) {
          resolve();
        } else if ([_jestWatcher().KEYS.ESCAPE, _jestWatcher().KEYS.CONTROL_C, _jestWatcher().KEYS.CONTROL_D].includes(key)) {
          reject();
        }
      });
    } else {
      resolve();
    }
  });
}

/***/ },

/***/ "./src/lib/isValidPath.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = isValidPath;
function _jestSnapshot() {
  const data = require("jest-snapshot");
  _jestSnapshot = function () {
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

function isValidPath(globalConfig, filePath) {
  return !filePath.includes(globalConfig.coverageDirectory) && !(0, _jestSnapshot().isSnapshotPath)(filePath);
}

/***/ },

/***/ "./src/lib/logDebugMessages.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = logDebugMessages;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const VERSION = (__webpack_require__("./package.json").version);

// if the output here changes, update `getConfig` in e2e/runJest.ts
function logDebugMessages(globalConfig, configs, outputStream) {
  const output = {
    configs,
    globalConfig: {
      ...globalConfig,
      testPathPatterns: globalConfig.testPathPatterns.patterns
    },
    version: VERSION
  };
  outputStream.write(`${JSON.stringify(output, null, '  ')}\n`);
}

/***/ },

/***/ "./src/lib/serializeToJSON.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = serializeToJSON;
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
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

/**
 * When we're asked to give a JSON output with the --json flag or otherwise,
 * some data we need to return don't serialize well with a basic
 * `JSON.stringify`, particularly Errors returned in `.openHandles`.
 *
 * This function handles the extended serialization wanted above.
 */
function serializeToJSON(value, space) {
  return JSON.stringify(value, (_, value) => {
    // There might be more in Error, but pulling out just the message, name,
    // and stack should be good enough
    if ((0, _jestUtil().isError)(value)) {
      return {
        message: value.message,
        name: value.name,
        stack: value.stack
      };
    }
    return value;
  }, space);
}

/***/ },

/***/ "./src/lib/updateGlobalConfig.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = updateGlobalConfig;
function _pattern() {
  const data = require("@jest/pattern");
  _pattern = function () {
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

function updateGlobalConfig(globalConfig, options = {}) {
  const newConfig = {
    ...globalConfig
  };
  if (options.mode === 'watch') {
    newConfig.watch = true;
    newConfig.watchAll = false;
  } else if (options.mode === 'watchAll') {
    newConfig.watch = false;
    newConfig.watchAll = true;
  }
  if (options.testNamePattern !== undefined) {
    newConfig.testNamePattern = options.testNamePattern || '';
  }
  if (options.testPathPatterns !== undefined) {
    newConfig.testPathPatterns = new (_pattern().TestPathPatterns)(options.testPathPatterns);
  }
  newConfig.onlyChanged = !newConfig.watchAll && !newConfig.testNamePattern && !newConfig.testPathPatterns.isSet();
  if (typeof options.bail === 'boolean') {
    newConfig.bail = options.bail ? 1 : 0;
  } else if (options.bail !== undefined) {
    newConfig.bail = options.bail;
  }
  if (options.changedSince !== undefined) {
    newConfig.changedSince = options.changedSince;
  }
  if (options.collectCoverage !== undefined) {
    newConfig.collectCoverage = options.collectCoverage || false;
  }
  if (options.collectCoverageFrom !== undefined) {
    newConfig.collectCoverageFrom = options.collectCoverageFrom;
  }
  if (options.coverageDirectory !== undefined) {
    newConfig.coverageDirectory = options.coverageDirectory;
  }
  if (options.coverageReporters !== undefined) {
    newConfig.coverageReporters = options.coverageReporters;
  }
  if (options.findRelatedTests !== undefined) {
    newConfig.findRelatedTests = options.findRelatedTests;
  }
  if (options.nonFlagArgs !== undefined) {
    newConfig.nonFlagArgs = options.nonFlagArgs;
  }
  if (options.noSCM) {
    newConfig.noSCM = true;
  }
  if (options.notify !== undefined) {
    newConfig.notify = options.notify || false;
  }
  if (options.notifyMode !== undefined) {
    newConfig.notifyMode = options.notifyMode;
  }
  if (options.onlyFailures !== undefined) {
    newConfig.onlyFailures = options.onlyFailures || false;
  }
  if (options.passWithNoTests !== undefined) {
    newConfig.passWithNoTests = true;
  }
  if (options.reporters !== undefined) {
    newConfig.reporters = options.reporters;
  }
  if (options.updateSnapshot !== undefined) {
    newConfig.updateSnapshot = options.updateSnapshot;
  }
  if (options.verbose !== undefined) {
    newConfig.verbose = options.verbose || false;
  }
  return Object.freeze(newConfig);
}

/***/ },

/***/ "./src/lib/watchPluginsHelpers.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.getSortedUsageRows = exports.filterInteractivePlugins = void 0;
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
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

const filterInteractivePlugins = (watchPlugins, globalConfig) => {
  const usageInfos = watchPlugins.map(p => p.getUsageInfo && p.getUsageInfo(globalConfig));
  return watchPlugins.filter((_plugin, i) => {
    const usageInfo = usageInfos[i];
    if (usageInfo) {
      const {
        key
      } = usageInfo;
      return !usageInfos.slice(i + 1).some(u => !!u && key === u.key);
    }
    return false;
  });
};
exports.filterInteractivePlugins = filterInteractivePlugins;
const getSortedUsageRows = (watchPlugins, globalConfig) => filterInteractivePlugins(watchPlugins, globalConfig).sort((a, b) => {
  if (a.isInternal && b.isInternal) {
    // internal plugins in the order we specify them
    return 0;
  }
  if (a.isInternal !== b.isInternal) {
    // external plugins afterwards
    return a.isInternal ? -1 : 1;
  }
  const usageInfoA = a.getUsageInfo && a.getUsageInfo(globalConfig);
  const usageInfoB = b.getUsageInfo && b.getUsageInfo(globalConfig);
  if (usageInfoA && usageInfoB) {
    // external plugins in alphabetical order
    return usageInfoA.key.localeCompare(usageInfoB.key);
  }
  return 0;
}).map(p => p.getUsageInfo && p.getUsageInfo(globalConfig)).filter(_jestUtil().isNonNullable);
exports.getSortedUsageRows = getSortedUsageRows;

/***/ },

/***/ "./src/plugins/FailedTestsInteractive.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
    return data;
  };
  return data;
}
var _FailedTestsInteractiveMode = _interopRequireDefault(__webpack_require__("./src/FailedTestsInteractiveMode.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class FailedTestsInteractivePlugin extends _jestWatcher().BaseWatchPlugin {
  _failedTestAssertions;
  _manager = new _FailedTestsInteractiveMode.default(this._stdout);
  apply(hooks) {
    hooks.onTestRunComplete(results => {
      this._failedTestAssertions = this.getFailedTestAssertions(results);
      if (this._manager.isActive()) this._manager.updateWithResults(results);
    });
  }
  getUsageInfo() {
    if (this._failedTestAssertions?.length) {
      return {
        key: 'i',
        prompt: 'run failing tests interactively'
      };
    }
    return null;
  }
  onKey(key) {
    if (this._manager.isActive()) {
      this._manager.put(key);
    }
  }
  run(_, updateConfigAndRun) {
    return new Promise(resolve => {
      if (!this._failedTestAssertions || this._failedTestAssertions.length === 0) {
        resolve();
        return;
      }
      this._manager.run(this._failedTestAssertions, failure => {
        updateConfigAndRun({
          mode: 'watch',
          testNamePattern: failure ? `^${failure.fullName}$` : '',
          testPathPatterns: failure ? [failure.path] : []
        });
        if (!this._manager.isActive()) {
          resolve();
        }
      });
    });
  }
  getFailedTestAssertions(results) {
    const failedTestPaths = [];
    if (
    // skip if no failed tests
    results.numFailedTests === 0 ||
    // skip if missing test results
    !results.testResults ||
    // skip if unmatched snapshots are present
    results.snapshot.unmatched) {
      return failedTestPaths;
    }
    for (const testResult of results.testResults) {
      for (const result of testResult.testResults) {
        if (result.status === 'failed') {
          failedTestPaths.push({
            fullName: result.fullName,
            path: testResult.testFilePath
          });
        }
      }
    }
    return failedTestPaths;
  }
}
exports["default"] = FailedTestsInteractivePlugin;

/***/ },

/***/ "./src/plugins/Quit.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
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

class QuitPlugin extends _jestWatcher().BaseWatchPlugin {
  isInternal;
  constructor(options) {
    super(options);
    this.isInternal = true;
  }
  async run() {
    if (typeof this._stdin.setRawMode === 'function') {
      this._stdin.setRawMode(false);
    }
    this._stdout.write('\n');
    process.exit(0);
  }
  getUsageInfo() {
    return {
      key: 'q',
      prompt: 'quit watch mode'
    };
  }
}
var _default = exports["default"] = QuitPlugin;

/***/ },

/***/ "./src/plugins/TestNamePattern.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
    return data;
  };
  return data;
}
var _TestNamePatternPrompt = _interopRequireDefault(__webpack_require__("./src/TestNamePatternPrompt.ts"));
var _activeFiltersMessage = _interopRequireDefault(__webpack_require__("./src/lib/activeFiltersMessage.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class TestNamePatternPlugin extends _jestWatcher().BaseWatchPlugin {
  _prompt;
  isInternal;
  constructor(options) {
    super(options);
    this._prompt = new (_jestWatcher().Prompt)();
    this.isInternal = true;
  }
  getUsageInfo() {
    return {
      key: 't',
      prompt: 'filter by a test name regex pattern'
    };
  }
  onKey(key) {
    this._prompt.put(key);
  }
  run(globalConfig, updateConfigAndRun) {
    return new Promise((resolve, reject) => {
      const testNamePatternPrompt = new _TestNamePatternPrompt.default(this._stdout, this._prompt);
      testNamePatternPrompt.run(value => {
        updateConfigAndRun({
          mode: 'watch',
          testNamePattern: value
        });
        resolve();
      }, reject, {
        header: (0, _activeFiltersMessage.default)(globalConfig)
      });
    });
  }
}
var _default = exports["default"] = TestNamePatternPlugin;

/***/ },

/***/ "./src/plugins/TestPathPattern.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
    return data;
  };
  return data;
}
var _TestPathPatternPrompt = _interopRequireDefault(__webpack_require__("./src/TestPathPatternPrompt.ts"));
var _activeFiltersMessage = _interopRequireDefault(__webpack_require__("./src/lib/activeFiltersMessage.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class TestPathPatternPlugin extends _jestWatcher().BaseWatchPlugin {
  _prompt;
  isInternal;
  constructor(options) {
    super(options);
    this._prompt = new (_jestWatcher().Prompt)();
    this.isInternal = true;
  }
  getUsageInfo() {
    return {
      key: 'p',
      prompt: 'filter by a filename regex pattern'
    };
  }
  onKey(key) {
    this._prompt.put(key);
  }
  run(globalConfig, updateConfigAndRun) {
    return new Promise((resolve, reject) => {
      const testPathPatternPrompt = new _TestPathPatternPrompt.default(this._stdout, this._prompt);
      testPathPatternPrompt.run(value => {
        updateConfigAndRun({
          mode: 'watch',
          testPathPatterns: [value]
        });
        resolve();
      }, reject, {
        header: (0, _activeFiltersMessage.default)(globalConfig)
      });
    });
  }
}
var _default = exports["default"] = TestPathPatternPlugin;

/***/ },

/***/ "./src/plugins/UpdateSnapshots.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
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

class UpdateSnapshotsPlugin extends _jestWatcher().BaseWatchPlugin {
  _hasSnapshotFailure;
  isInternal;
  constructor(options) {
    super(options);
    this.isInternal = true;
    this._hasSnapshotFailure = false;
  }
  run(_globalConfig, updateConfigAndRun) {
    updateConfigAndRun({
      updateSnapshot: 'all'
    });
    return Promise.resolve(false);
  }
  apply(hooks) {
    hooks.onTestRunComplete(results => {
      this._hasSnapshotFailure = results.snapshot.failure;
    });
  }
  getUsageInfo() {
    if (this._hasSnapshotFailure) {
      return {
        key: 'u',
        prompt: 'update failing snapshots'
      };
    }
    return null;
  }
}
var _default = exports["default"] = UpdateSnapshotsPlugin;

/***/ },

/***/ "./src/plugins/UpdateSnapshotsInteractive.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
    return data;
  };
  return data;
}
var _SnapshotInteractiveMode = _interopRequireDefault(__webpack_require__("./src/SnapshotInteractiveMode.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class UpdateSnapshotInteractivePlugin extends _jestWatcher().BaseWatchPlugin {
  _snapshotInteractiveMode = new _SnapshotInteractiveMode.default(this._stdout);
  _failedSnapshotTestAssertions = [];
  isInternal = true;
  getFailedSnapshotTestAssertions(testResults) {
    const failedTestPaths = [];
    if (testResults.numFailedTests === 0 || !testResults.testResults) {
      return failedTestPaths;
    }
    for (const testResult of testResults.testResults) {
      if (testResult.snapshot && testResult.snapshot.unmatched) {
        for (const result of testResult.testResults) {
          if (result.status === 'failed') {
            failedTestPaths.push({
              fullName: result.fullName,
              path: testResult.testFilePath
            });
          }
        }
      }
    }
    return failedTestPaths;
  }
  apply(hooks) {
    hooks.onTestRunComplete(results => {
      this._failedSnapshotTestAssertions = this.getFailedSnapshotTestAssertions(results);
      if (this._snapshotInteractiveMode.isActive()) {
        this._snapshotInteractiveMode.updateWithResults(results);
      }
    });
  }
  onKey(key) {
    if (this._snapshotInteractiveMode.isActive()) {
      this._snapshotInteractiveMode.put(key);
    }
  }
  run(_globalConfig, updateConfigAndRun) {
    if (this._failedSnapshotTestAssertions.length > 0) {
      return new Promise(resolve => {
        this._snapshotInteractiveMode.run(this._failedSnapshotTestAssertions, (assertion, shouldUpdateSnapshot) => {
          updateConfigAndRun({
            mode: 'watch',
            testNamePattern: assertion ? `^${assertion.fullName}$` : '',
            testPathPatterns: assertion ? [assertion.path] : [],
            updateSnapshot: shouldUpdateSnapshot ? 'all' : 'none'
          });
          if (!this._snapshotInteractiveMode.isActive()) {
            resolve();
          }
        });
      });
    } else {
      return Promise.resolve();
    }
  }
  getUsageInfo() {
    if (this._failedSnapshotTestAssertions?.length > 0) {
      return {
        key: 'i',
        prompt: 'update failing snapshots interactively'
      };
    }
    return null;
  }
}
var _default = exports["default"] = UpdateSnapshotInteractivePlugin;

/***/ },

/***/ "./src/runGlobalHook.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = runGlobalHook;
function _transform() {
  const data = require("@jest/transform");
  _transform = function () {
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
function _prettyFormat() {
  const data = _interopRequireDefault(require("pretty-format"));
  _prettyFormat = function () {
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

async function runGlobalHook({
  allTests,
  globalConfig,
  moduleName
}) {
  const globalModulePaths = new Set(allTests.map(test => test.context.config[moduleName]));
  if (globalConfig[moduleName]) {
    globalModulePaths.add(globalConfig[moduleName]);
  }
  if (globalModulePaths.size > 0) {
    for (const modulePath of globalModulePaths) {
      if (!modulePath) {
        continue;
      }
      const correctConfig = allTests.find(t => t.context.config[moduleName] === modulePath);
      const projectConfig = correctConfig ? correctConfig.context.config :
      // Fallback to first config
      allTests[0].context.config;
      const transformer = await (0, _transform().createScriptTransformer)(projectConfig);
      try {
        await transformer.requireAndTranspileModule(modulePath, async globalModule => {
          if (typeof globalModule !== 'function') {
            throw new TypeError(`${moduleName} file must export a function at ${modulePath}`);
          }
          await globalModule(globalConfig, projectConfig);
        });
      } catch (error) {
        if ((0, _jestUtil().isError)(error) && (Object.getOwnPropertyDescriptor(error, 'message')?.writable || Object.getOwnPropertyDescriptor(Object.getPrototypeOf(error), 'message')?.writable)) {
          error.message = `Jest: Got error running ${moduleName} - ${modulePath}, reason: ${error.message}`;
          throw error;
        }
        throw new Error(`Jest: Got error running ${moduleName} - ${modulePath}, reason: ${(0, _prettyFormat().default)(error, {
          maxDepth: 3
        })}`);
      }
    }
  }
}

/***/ },

/***/ "./src/runJest.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = runJest;
exports.printCollectedTestTree = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _nodePerf_hooks() {
  const data = require("node:perf_hooks");
  _nodePerf_hooks = function () {
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
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
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
function _reporters() {
  const data = require("@jest/reporters");
  _reporters = function () {
    return data;
  };
  return data;
}
function _testResult() {
  const data = require("@jest/test-result");
  _testResult = function () {
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
    return data;
  };
  return data;
}
var _SearchSource = _interopRequireDefault(__webpack_require__("./src/SearchSource.ts"));
var _TestScheduler = __webpack_require__("./src/TestScheduler.ts");
var _collectHandles = _interopRequireDefault(__webpack_require__("./src/collectHandles.ts"));
var _getNoTestsFoundMessage = _interopRequireDefault(__webpack_require__("./src/getNoTestsFoundMessage.ts"));
var _serializeToJSON = _interopRequireDefault(__webpack_require__("./src/lib/serializeToJSON.ts"));
var _runGlobalHook = _interopRequireDefault(__webpack_require__("./src/runGlobalHook.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const printCollectedTestTree = (testResults, outputStream) => {
  const printSuite = (suite, indent) => {
    if (suite.title) {
      outputStream.write(`${'  '.repeat(indent)}${suite.title}\n`);
    }
    for (const t of suite.tests) {
      outputStream.write(`${'  '.repeat(indent + 1)}${t.title}\n`);
    }
    for (const child of suite.suites) {
      printSuite(child, indent + 1);
    }
  };
  const root = _reporters().VerboseReporter.groupTestsBySuites(testResults);
  printSuite(root, 0);
};
exports.printCollectedTestTree = printCollectedTestTree;
const getTestPaths = async (globalConfig, projectConfig, source, outputStream, changedFiles, jestHooks, filter) => {
  const data = await source.getTestPaths(globalConfig, projectConfig, changedFiles, filter);
  if (data.tests.length === 0 && globalConfig.onlyChanged && data.noSCM) {
    new (_console().CustomConsole)(outputStream, outputStream).log('Jest can only find uncommitted changed files in a git or hg ' + 'repository. If you make your project a git or hg ' + 'repository (`git init` or `hg init`), Jest will be able ' + 'to only run tests related to files changed since the last ' + 'commit.');
  }
  const shouldTestArray = await Promise.all(data.tests.map(test => jestHooks.shouldRunTestSuite({
    config: test.context.config,
    duration: test.duration,
    testPath: test.path
  })));
  const filteredTests = data.tests.filter((_test, i) => shouldTestArray[i]);
  return {
    ...data,
    allTests: filteredTests.length,
    tests: filteredTests
  };
};
const processResults = async (runResults, options) => {
  const {
    outputFile,
    json: isJSON,
    onComplete,
    outputStream,
    testResultsProcessor,
    collectHandles
  } = options;
  if (collectHandles) {
    runResults.openHandles = await collectHandles();
  } else {
    runResults.openHandles = [];
  }
  if (testResultsProcessor) {
    const processor = await (0, _jestUtil().requireOrImportModule)(testResultsProcessor);
    runResults = await processor(runResults);
  }
  if (isJSON) {
    const jsonString = (0, _serializeToJSON.default)((0, _testResult().formatTestResults)(runResults));
    if (outputFile) {
      const cwd = (0, _jestUtil().tryRealpath)(process.cwd());
      const filePath = path().resolve(cwd, outputFile);
      fs().writeFileSync(filePath, `${jsonString}\n`);
      outputStream.write(`Test results written to: ${path().relative(cwd, filePath)}\n`);
    } else {
      process.stdout.write(`${jsonString}\n`);
    }
  }
  onComplete?.(runResults);
};
const testSchedulerContext = {
  firstRun: true,
  previousSuccess: true
};
async function runJest({
  contexts,
  globalConfig,
  outputStream,
  testWatcher,
  jestHooks = new (_jestWatcher().JestHook)().getEmitter(),
  startRun,
  changedFilesPromise,
  onComplete,
  failedTestsCache,
  filter
}) {
  // Clear cache for required modules - there might be different resolutions
  // from Jest's config loading to running the tests
  _jestResolve().default.clearDefaultResolverCache();
  const Sequencer = await (0, _jestUtil().requireOrImportModule)(globalConfig.testSequencer);
  const sequencer = new Sequencer({
    contexts,
    globalConfig
  });
  let allTests = [];
  if (changedFilesPromise && globalConfig.watch) {
    const {
      repos
    } = await changedFilesPromise;
    const noSCM = Object.keys(repos).every(scm => repos[scm].size === 0);
    if (noSCM) {
      process.stderr.write(`\n${_chalk().default.bold('--watch')} is not supported without git/hg, please use --watchAll\n`);
      (0, _exitX().default)(1);
    }
  }
  const searchSources = contexts.map(context => new _SearchSource.default(context));
  _nodePerf_hooks().performance.mark('jest/getTestPaths:start');
  const testRunData = await Promise.all(contexts.map(async (context, index) => {
    const searchSource = searchSources[index];
    const matches = await getTestPaths(globalConfig, context.config, searchSource, outputStream, changedFilesPromise && (await changedFilesPromise), jestHooks, filter);
    allTests = [...allTests, ...matches.tests];
    return {
      context,
      matches
    };
  }));
  _nodePerf_hooks().performance.mark('jest/getTestPaths:end');
  if (globalConfig.shard) {
    if (typeof sequencer.shard !== 'function') {
      throw new TypeError(`Shard ${globalConfig.shard.shardIndex}/${globalConfig.shard.shardCount} requested, but test sequencer ${Sequencer.name} in ${globalConfig.testSequencer} has no shard method.`);
    }
    allTests = await sequencer.shard(allTests, globalConfig.shard);
  }
  allTests = await sequencer.sort(allTests);
  if (globalConfig.onlyFailures) {
    if (failedTestsCache) {
      allTests = failedTestsCache.filterTests(allTests);
    } else {
      allTests = await sequencer.allFailedTests(allTests);
    }
  }
  if (globalConfig.listTests) {
    const testsPaths = [...new Set(allTests.map(test => test.path))];
    let testsListOutput;
    if (globalConfig.json) {
      testsListOutput = JSON.stringify(testsPaths);
    } else {
      testsListOutput = testsPaths.join('\n');
    }
    if (globalConfig.outputFile) {
      const outputFile = path().resolve(process.cwd(), globalConfig.outputFile);
      fs().writeFileSync(outputFile, testsListOutput, 'utf8');
    } else {
      // eslint-disable-next-line no-console
      console.log(testsListOutput);
    }
    onComplete?.((0, _testResult().makeEmptyAggregatedTestResult)());
    return;
  }
  const hasTests = allTests.length > 0;
  if (globalConfig.collectTests) {
    if (!hasTests) {
      // eslint-disable-next-line no-console
      console.log('No tests found.');
      onComplete?.((0, _testResult().makeEmptyAggregatedTestResult)());
      return;
    }

    // Suppress reporters; circus collects tests without executing.
    const collectTestsConfig = Object.freeze({
      ...globalConfig,
      collectCoverage: false,
      reporters: [],
      silent: true
    });
    const scheduler = await (0, _TestScheduler.createTestScheduler)(collectTestsConfig, {
      startRun,
      ...testSchedulerContext
    });
    const results = await scheduler.scheduleTests(allTests, testWatcher);
    if (!globalConfig.json) {
      for (const testResult of results.testResults) {
        if (testResult.testResults.length > 0) {
          outputStream.write(`${testResult.testFilePath}\n`);
          printCollectedTestTree(testResult.testResults, outputStream);
        }
      }
    }
    await processResults(results, {
      json: globalConfig.json,
      onComplete,
      outputFile: globalConfig.outputFile,
      outputStream,
      testResultsProcessor: globalConfig.testResultsProcessor
    });
    return;
  }
  if (!hasTests) {
    const {
      exitWith0,
      message: noTestsFoundMessage
    } = (0, _getNoTestsFoundMessage.default)(testRunData, globalConfig);
    if (exitWith0) {
      new (_console().CustomConsole)(outputStream, outputStream).log(noTestsFoundMessage);
    } else {
      new (_console().CustomConsole)(outputStream, outputStream).error(noTestsFoundMessage);
      (0, _exitX().default)(1);
    }
  } else if (allTests.length === 1 && globalConfig.silent !== true && globalConfig.verbose !== false) {
    const newConfig = {
      ...globalConfig,
      verbose: true
    };
    globalConfig = Object.freeze(newConfig);
  }
  let collectHandles;
  if (globalConfig.detectOpenHandles) {
    collectHandles = (0, _collectHandles.default)();
  }
  if (hasTests) {
    _nodePerf_hooks().performance.mark('jest/globalSetup:start');
    await (0, _runGlobalHook.default)({
      allTests,
      globalConfig,
      moduleName: 'globalSetup'
    });
    _nodePerf_hooks().performance.mark('jest/globalSetup:end');
  }
  if (changedFilesPromise) {
    const changedFilesInfo = await changedFilesPromise;
    if (changedFilesInfo.changedFiles) {
      testSchedulerContext.changedFiles = changedFilesInfo.changedFiles;
      const relatedFiles = await Promise.all(contexts.map(async (_, index) => {
        const searchSource = searchSources[index];
        return searchSource.findRelatedSourcesFromTestsInChangedFiles(changedFilesInfo);
      }));
      const sourcesRelatedToTestsInChangedFilesArray = relatedFiles.flat();
      testSchedulerContext.sourcesRelatedToTestsInChangedFiles = new Set(sourcesRelatedToTestsInChangedFilesArray);
    }
  }
  const scheduler = await (0, _TestScheduler.createTestScheduler)(globalConfig, {
    startRun,
    ...testSchedulerContext
  });
  _nodePerf_hooks().performance.mark('jest/scheduleAndRun:start', {
    detail: {
      numTests: allTests.length
    }
  });
  const results = await scheduler.scheduleTests(allTests, testWatcher);
  _nodePerf_hooks().performance.mark('jest/scheduleAndRun:end');
  _nodePerf_hooks().performance.mark('jest/cacheResults:start');
  sequencer.cacheResults(allTests, results);
  _nodePerf_hooks().performance.mark('jest/cacheResults:end');
  if (hasTests) {
    _nodePerf_hooks().performance.mark('jest/globalTeardown:start');
    await (0, _runGlobalHook.default)({
      allTests,
      globalConfig,
      moduleName: 'globalTeardown'
    });
    _nodePerf_hooks().performance.mark('jest/globalTeardown:end');
  }
  _nodePerf_hooks().performance.mark('jest/processResults:start');
  await processResults(results, {
    collectHandles,
    json: globalConfig.json,
    onComplete,
    outputFile: globalConfig.outputFile,
    outputStream,
    testResultsProcessor: globalConfig.testResultsProcessor
  });
  _nodePerf_hooks().performance.mark('jest/processResults:end');
}

/***/ },

/***/ "./src/testSchedulerHelper.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.shouldRunInBand = shouldRunInBand;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const SLOW_TEST_TIME = 1000;
function shouldRunInBand(tests, timings, {
  detectOpenHandles,
  maxWorkers,
  runInBand,
  watch,
  watchAll,
  workerIdleMemoryLimit
}) {
  // If user asked for run in band, respect that.
  // detectOpenHandles makes no sense without runInBand, because it cannot detect leaks in workers
  if (runInBand || detectOpenHandles) {
    return true;
  }

  /*
   * If we are using watch/watchAll mode, don't schedule anything in the main
   * thread to keep the TTY responsive and to prevent watch mode crashes caused
   * by leaks (improper test teardown).
   */
  if (watch || watchAll) {
    return false;
  }

  /*
   * Otherwise, run in band if we only have one test or one worker available.
   * Also, if we are confident from previous runs that the tests will finish
   * quickly we also run in band to reduce the overhead of spawning workers.
   */
  const areFastTests = timings.every(timing => timing < SLOW_TEST_TIME);
  const oneWorkerOrLess = maxWorkers <= 1;
  const oneTestOrLess = tests.length <= 1;
  return (
    // When specifying a memory limit, workers should be used
    workerIdleMemoryLimit === undefined && (oneWorkerOrLess || oneTestOrLess || tests.length <= 20 && timings.length > 0 && areFastTests)
  );
}

/***/ },

/***/ "./src/version.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getVersion;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Cannot be `import` as it's not under TS root dir
const {
  version: VERSION
} = __webpack_require__("./package.json");
function getVersion() {
  return VERSION;
}

/***/ },

/***/ "./src/watch.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = watch;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _ansiEscapes() {
  const data = _interopRequireDefault(require("ansi-escapes"));
  _ansiEscapes = function () {
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
function _slash() {
  const data = _interopRequireDefault(require("slash"));
  _slash = function () {
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
function _jestMessageUtil() {
  const data = require("jest-message-util");
  _jestMessageUtil = function () {
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
function _jestWatcher() {
  const data = require("jest-watcher");
  _jestWatcher = function () {
    return data;
  };
  return data;
}
var _FailedTestsCache = _interopRequireDefault(__webpack_require__("./src/FailedTestsCache.ts"));
var _SearchSource = _interopRequireDefault(__webpack_require__("./src/SearchSource.ts"));
var _getChangedFilesPromise = _interopRequireDefault(__webpack_require__("./src/getChangedFilesPromise.ts"));
var _activeFiltersMessage = _interopRequireDefault(__webpack_require__("./src/lib/activeFiltersMessage.ts"));
var _createContext = _interopRequireDefault(__webpack_require__("./src/lib/createContext.ts"));
var _isValidPath = _interopRequireDefault(__webpack_require__("./src/lib/isValidPath.ts"));
var _updateGlobalConfig = _interopRequireDefault(__webpack_require__("./src/lib/updateGlobalConfig.ts"));
var _watchPluginsHelpers = __webpack_require__("./src/lib/watchPluginsHelpers.ts");
var _FailedTestsInteractive = _interopRequireDefault(__webpack_require__("./src/plugins/FailedTestsInteractive.ts"));
var _Quit = _interopRequireDefault(__webpack_require__("./src/plugins/Quit.ts"));
var _TestNamePattern = _interopRequireDefault(__webpack_require__("./src/plugins/TestNamePattern.ts"));
var _TestPathPattern = _interopRequireDefault(__webpack_require__("./src/plugins/TestPathPattern.ts"));
var _UpdateSnapshots = _interopRequireDefault(__webpack_require__("./src/plugins/UpdateSnapshots.ts"));
var _UpdateSnapshotsInteractive = _interopRequireDefault(__webpack_require__("./src/plugins/UpdateSnapshotsInteractive.ts"));
var _runJest = _interopRequireDefault(__webpack_require__("./src/runJest.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const {
  print: preRunMessagePrint
} = _jestUtil().preRunMessage;
let hasExitListener = false;
const INTERNAL_PLUGINS = [_FailedTestsInteractive.default, _TestPathPattern.default, _TestNamePattern.default, _UpdateSnapshots.default, _UpdateSnapshotsInteractive.default, _Quit.default];
const RESERVED_KEY_PLUGINS = new Map([[_UpdateSnapshots.default, {
  forbiddenOverwriteMessage: 'updating snapshots',
  key: 'u'
}], [_UpdateSnapshotsInteractive.default, {
  forbiddenOverwriteMessage: 'updating snapshots interactively',
  key: 'i'
}], [_Quit.default, {
  forbiddenOverwriteMessage: 'quitting watch mode'
}]]);
async function watch(initialGlobalConfig, contexts, outputStream, hasteMapInstances, stdin = process.stdin, hooks = new (_jestWatcher().JestHook)(), filter) {
  // `globalConfig` will be constantly updated and reassigned as a result of
  // watch mode interactions.
  let globalConfig = initialGlobalConfig;
  let activePlugin;
  globalConfig = (0, _updateGlobalConfig.default)(globalConfig, {
    mode: globalConfig.watch ? 'watch' : 'watchAll',
    passWithNoTests: true
  });
  const updateConfigAndRun = async ({
    bail,
    changedSince,
    collectCoverage,
    collectCoverageFrom,
    coverageDirectory,
    coverageReporters,
    findRelatedTests,
    mode,
    nonFlagArgs,
    notify,
    notifyMode,
    onlyFailures,
    reporters,
    testNamePattern,
    testPathPatterns,
    updateSnapshot,
    verbose
  } = {}) => {
    const previousUpdateSnapshot = globalConfig.updateSnapshot;
    globalConfig = (0, _updateGlobalConfig.default)(globalConfig, {
      bail,
      changedSince,
      collectCoverage,
      collectCoverageFrom,
      coverageDirectory,
      coverageReporters,
      findRelatedTests,
      mode,
      nonFlagArgs,
      notify,
      notifyMode,
      onlyFailures,
      reporters,
      testNamePattern,
      testPathPatterns,
      updateSnapshot,
      verbose
    });
    startRun(globalConfig);
    globalConfig = (0, _updateGlobalConfig.default)(globalConfig, {
      // updateSnapshot is not sticky after a run.
      updateSnapshot: previousUpdateSnapshot === 'all' ? 'none' : previousUpdateSnapshot
    });
  };
  const watchPlugins = INTERNAL_PLUGINS.map(InternalPlugin => new InternalPlugin({
    stdin,
    stdout: outputStream
  }));
  for (const plugin of watchPlugins) {
    const hookSubscriber = hooks.getSubscriber();
    if (plugin.apply) {
      plugin.apply(hookSubscriber);
    }
  }
  if (globalConfig.watchPlugins != null) {
    const watchPluginKeys = new Map();
    for (const plugin of watchPlugins) {
      const reservedInfo = RESERVED_KEY_PLUGINS.get(plugin.constructor) || {};
      const key = reservedInfo.key || getPluginKey(plugin, globalConfig);
      if (!key) {
        continue;
      }
      const {
        forbiddenOverwriteMessage
      } = reservedInfo;
      watchPluginKeys.set(key, {
        forbiddenOverwriteMessage,
        overwritable: forbiddenOverwriteMessage == null,
        plugin
      });
    }
    for (const pluginWithConfig of globalConfig.watchPlugins) {
      let plugin;
      try {
        const ThirdPartyPlugin = await (0, _jestUtil().requireOrImportModule)(pluginWithConfig.path);
        plugin = new ThirdPartyPlugin({
          config: pluginWithConfig.config,
          stdin,
          stdout: outputStream
        });
      } catch (error) {
        const errorWithContext = new Error(`Failed to initialize watch plugin "${_chalk().default.bold((0, _slash().default)(path().relative(process.cwd(), pluginWithConfig.path)))}":\n\n${(0, _jestMessageUtil().formatExecError)(error, contexts[0].config, {
          noStackTrace: false
        })}`);
        delete errorWithContext.stack;
        throw errorWithContext;
      }
      checkForConflicts(watchPluginKeys, plugin, globalConfig);
      const hookSubscriber = hooks.getSubscriber();
      if (plugin.apply) {
        plugin.apply(hookSubscriber);
      }
      watchPlugins.push(plugin);
    }
  }
  const failedTestsCache = new _FailedTestsCache.default();
  let searchSources = contexts.map(context => ({
    context,
    searchSource: new _SearchSource.default(context)
  }));
  let isRunning = false;
  let testWatcher;
  let shouldDisplayWatchUsage = true;
  let isWatchUsageDisplayed = false;
  const emitFileChange = () => {
    if (hooks.isUsed('onFileChange')) {
      const projects = searchSources.map(({
        context,
        searchSource
      }) => ({
        config: context.config,
        testPaths: searchSource.findMatchingTests(new (_pattern().TestPathPatterns)([]).toExecutor({
          rootDir: context.config.rootDir
        })).tests.map(t => t.path)
      }));
      hooks.getEmitter().onFileChange({
        projects
      });
    }
  };
  emitFileChange();
  for (const [index, hasteMapInstance] of hasteMapInstances.entries()) {
    hasteMapInstance.on('change', ({
      eventsQueue,
      hasteFS,
      moduleMap
    }) => {
      const validPaths = eventsQueue.filter(({
        filePath
      }) => (0, _isValidPath.default)(globalConfig, filePath));
      if (validPaths.length > 0) {
        const context = contexts[index] = (0, _createContext.default)(contexts[index].config, {
          hasteFS,
          moduleMap
        });
        activePlugin = null;
        searchSources = [...searchSources];
        searchSources[index] = {
          context,
          searchSource: new _SearchSource.default(context)
        };
        emitFileChange();
        startRun(globalConfig);
      }
    });
  }
  if (!hasExitListener) {
    hasExitListener = true;
    process.on('exit', () => {
      if (activePlugin) {
        outputStream.write(_ansiEscapes().default.cursorDown());
        outputStream.write(_ansiEscapes().default.eraseDown);
      }
    });
  }
  const startRun = async globalConfig => {
    if (isRunning) {
      return;
    }
    testWatcher = new (_jestWatcher().TestWatcher)({
      isWatchMode: true
    });
    if (_jestUtil().isInteractive) {
      outputStream.write(_jestUtil().specialChars.CLEAR);
    }
    preRunMessagePrint(outputStream);
    isRunning = true;
    const configs = contexts.map(context => context.config);
    const changedFilesPromise = (0, _getChangedFilesPromise.default)(globalConfig, configs);
    try {
      await (0, _runJest.default)({
        changedFilesPromise,
        contexts,
        failedTestsCache,
        filter,
        globalConfig,
        jestHooks: hooks.getEmitter(),
        onComplete: results => {
          isRunning = false;
          hooks.getEmitter().onTestRunComplete(results);

          // Create a new testWatcher instance so that re-runs won't be blocked.
          // The old instance that was passed to Jest will still be interrupted
          // and prevent test runs from the previous run.
          testWatcher = new (_jestWatcher().TestWatcher)({
            isWatchMode: true
          });

          // Do not show any Watch Usage related stuff when running in a
          // non-interactive environment
          if (_jestUtil().isInteractive) {
            if (shouldDisplayWatchUsage) {
              outputStream.write(usage(globalConfig, watchPlugins));
              shouldDisplayWatchUsage = false; // hide Watch Usage after first run
              isWatchUsageDisplayed = true;
            } else {
              outputStream.write(showToggleUsagePrompt());
              shouldDisplayWatchUsage = false;
              isWatchUsageDisplayed = false;
            }
          } else {
            outputStream.write('\n');
          }
          failedTestsCache.setTestResults(results.testResults);
        },
        outputStream,
        startRun,
        testWatcher
      });
    } catch (error) {
      // Errors thrown inside `runJest`, e.g. by resolvers, are caught here for
      // continuous watch mode execution. We need to reprint them to the
      // terminal and give just a little bit of extra space so they fit below
      // `preRunMessagePrint` message nicely.
      console.error(`\n\n${(0, _jestMessageUtil().formatExecError)(error, contexts[0].config, {
        noStackTrace: false
      })}`);
    }
  };
  const onKeypress = key => {
    if (key === _jestWatcher().KEYS.CONTROL_C || key === _jestWatcher().KEYS.CONTROL_D) {
      if (typeof stdin.setRawMode === 'function') {
        stdin.setRawMode(false);
      }
      outputStream.write('\n');
      (0, _exitX().default)(0);
      return;
    }
    if (activePlugin != null && activePlugin.onKey) {
      // if a plugin is activate, Jest should let it handle keystrokes, so ignore
      // them here
      activePlugin.onKey(key);
      return;
    }

    // Abort test run
    const pluginKeys = (0, _watchPluginsHelpers.getSortedUsageRows)(watchPlugins, globalConfig).map(usage => Number(usage.key).toString(16));
    if (isRunning && testWatcher && ['q', _jestWatcher().KEYS.ENTER, 'a', 'o', 'f', ...pluginKeys].includes(key)) {
      testWatcher.setState({
        interrupted: true
      });
      return;
    }
    const matchingWatchPlugin = (0, _watchPluginsHelpers.filterInteractivePlugins)(watchPlugins, globalConfig).find(plugin => getPluginKey(plugin, globalConfig) === key);
    if (matchingWatchPlugin != null) {
      if (isRunning) {
        testWatcher.setState({
          interrupted: true
        });
        return;
      }
      // "activate" the plugin, which has jest ignore keystrokes so the plugin
      // can handle them
      activePlugin = matchingWatchPlugin;
      if (activePlugin.run) {
        activePlugin.run(globalConfig, updateConfigAndRun).then(async shouldRerun => {
          activePlugin = null;
          if (shouldRerun) {
            await updateConfigAndRun();
          }
        }, () => {
          activePlugin = null;
          onCancelPatternPrompt();
        });
      } else {
        activePlugin = null;
      }
    }
    switch (key) {
      case _jestWatcher().KEYS.ENTER:
        startRun(globalConfig);
        break;
      case 'a':
        globalConfig = (0, _updateGlobalConfig.default)(globalConfig, {
          mode: 'watchAll',
          testNamePattern: '',
          testPathPatterns: []
        });
        startRun(globalConfig);
        break;
      case 'c':
        updateConfigAndRun({
          mode: 'watch',
          testNamePattern: '',
          testPathPatterns: []
        });
        break;
      case 'f':
        globalConfig = (0, _updateGlobalConfig.default)(globalConfig, {
          onlyFailures: !globalConfig.onlyFailures
        });
        startRun(globalConfig);
        break;
      case 'o':
        globalConfig = (0, _updateGlobalConfig.default)(globalConfig, {
          mode: 'watch',
          testNamePattern: '',
          testPathPatterns: []
        });
        startRun(globalConfig);
        break;
      case '?':
        break;
      case 'w':
        if (!shouldDisplayWatchUsage && !isWatchUsageDisplayed) {
          outputStream.write(_ansiEscapes().default.cursorUp());
          outputStream.write(_ansiEscapes().default.eraseDown);
          outputStream.write(usage(globalConfig, watchPlugins));
          isWatchUsageDisplayed = true;
          shouldDisplayWatchUsage = false;
        }
        break;
    }
  };
  const onCancelPatternPrompt = () => {
    outputStream.write(_ansiEscapes().default.cursorHide);
    outputStream.write(_jestUtil().specialChars.CLEAR);
    outputStream.write(usage(globalConfig, watchPlugins));
    outputStream.write(_ansiEscapes().default.cursorShow);
  };
  if (typeof stdin.setRawMode === 'function') {
    stdin.setRawMode(true);
    stdin.resume();
    stdin.setEncoding('utf8');
    stdin.on('data', onKeypress);
  }
  startRun(globalConfig);
}
const checkForConflicts = (watchPluginKeys, plugin, globalConfig) => {
  const key = getPluginKey(plugin, globalConfig);
  if (!key) {
    return;
  }
  const conflictor = watchPluginKeys.get(key);
  if (!conflictor || conflictor.overwritable) {
    watchPluginKeys.set(key, {
      overwritable: false,
      plugin
    });
    return;
  }
  let error;
  if (conflictor.forbiddenOverwriteMessage) {
    error = `
  Watch plugin ${_chalk().default.bold.red(getPluginIdentifier(plugin))} attempted to register key ${_chalk().default.bold.red(`<${key}>`)},
  that is reserved internally for ${_chalk().default.bold.red(conflictor.forbiddenOverwriteMessage)}.
  Please change the configuration key for this plugin.`.trim();
  } else {
    const plugins = [conflictor.plugin, plugin].map(p => _chalk().default.bold.red(getPluginIdentifier(p))).join(' and ');
    error = `
  Watch plugins ${plugins} both attempted to register key ${_chalk().default.bold.red(`<${key}>`)}.
  Please change the key configuration for one of the conflicting plugins to avoid overlap.`.trim();
  }
  throw new (_jestValidate().ValidationError)('Watch plugin configuration error', error);
};
const getPluginIdentifier = plugin =>
// This breaks as `displayName` is not defined as a static, but since
// WatchPlugin is an interface, and it is my understanding interface
// static fields are not definable anymore, no idea how to circumvent
// this :-(
// @ts-expect-error: leave `displayName` be.
plugin.constructor.displayName || plugin.constructor.name;
const getPluginKey = (plugin, globalConfig) => {
  if (typeof plugin.getUsageInfo === 'function') {
    return (plugin.getUsageInfo(globalConfig) || {
      key: null
    }).key;
  }
  return null;
};
const usage = (globalConfig, watchPlugins, delimiter = '\n') => {
  const testPathPatterns = globalConfig.testPathPatterns;
  const messages = [(0, _activeFiltersMessage.default)(globalConfig), testPathPatterns.isSet() || globalConfig.testNamePattern ? `${_chalk().default.dim(' \u203A Press ')}c${_chalk().default.dim(' to clear filters.')}` : null, `\n${_chalk().default.bold('Watch Usage')}`, globalConfig.watch ? `${_chalk().default.dim(' \u203A Press ')}a${_chalk().default.dim(' to run all tests.')}` : null, globalConfig.onlyFailures ? `${_chalk().default.dim(' \u203A Press ')}f${_chalk().default.dim(' to quit "only failed tests" mode.')}` : `${_chalk().default.dim(' \u203A Press ')}f${_chalk().default.dim(' to run only failed tests.')}`, (globalConfig.watchAll || testPathPatterns.isSet() || globalConfig.testNamePattern) && !globalConfig.noSCM ? `${_chalk().default.dim(' \u203A Press ')}o${_chalk().default.dim(' to only run tests related to changed files.')}` : null, ...(0, _watchPluginsHelpers.getSortedUsageRows)(watchPlugins, globalConfig).map(plugin => `${_chalk().default.dim(' \u203A Press')} ${plugin.key} ${_chalk().default.dim(`to ${plugin.prompt}.`)}`), `${_chalk().default.dim(' \u203A Press ')}Enter${_chalk().default.dim(' to trigger a test run.')}`];
  return `${messages.filter(message => !!message).join(delimiter)}\n`;
};
const showToggleUsagePrompt = () => '\n' + `${_chalk().default.bold('Watch Usage: ')}${_chalk().default.dim('Press ')}w${_chalk().default.dim(' to show more.')}`;

/***/ },

/***/ "./package.json"
(module) {

module.exports = {"version":"30.4.1"};

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
Object.defineProperty(exports, "SearchSource", ({
  enumerable: true,
  get: function () {
    return _SearchSource.default;
  }
}));
Object.defineProperty(exports, "createTestScheduler", ({
  enumerable: true,
  get: function () {
    return _TestScheduler.createTestScheduler;
  }
}));
Object.defineProperty(exports, "getVersion", ({
  enumerable: true,
  get: function () {
    return _version.default;
  }
}));
Object.defineProperty(exports, "runCLI", ({
  enumerable: true,
  get: function () {
    return _cli.runCLI;
  }
}));
var _SearchSource = _interopRequireDefault(__webpack_require__("./src/SearchSource.ts"));
var _TestScheduler = __webpack_require__("./src/TestScheduler.ts");
var _cli = __webpack_require__("./src/cli/index.ts");
var _version = _interopRequireDefault(__webpack_require__("./src/version.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
})();

module.exports = __webpack_exports__;
/******/ })()
;