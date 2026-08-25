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

/***/ "./src/AgentReporter.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _BaseReporter = _interopRequireDefault(__webpack_require__("./src/BaseReporter.ts"));
var _DefaultReporter = _interopRequireDefault(__webpack_require__("./src/DefaultReporter.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/**
 * A reporter optimized for AI coding agents that reduces token usage by only
 * printing failing tests and the final summary. Automatically activated when
 * an AI agent environment is detected (via the AI_AGENT env var or std-env).
 */
class AgentReporter extends _DefaultReporter.default {
  static filename = __filename;

  /* eslint-disable @typescript-eslint/no-empty-function */
  __wrapStdio() {}
  __clearStatus() {}
  __printStatus() {}
  onTestStart(_test) {}
  onTestCaseResult(_test, _testCaseResult) {}
  /* eslint-enable */

  onRunStart(aggregatedResults, options) {
    _BaseReporter.default.prototype.onRunStart.call(this, aggregatedResults, options);
  }
  onTestResult(test, testResult, aggregatedResults) {
    this.testFinished(test.context.config, testResult, aggregatedResults);

    // Only print output for test files that have failures.
    if (!testResult.skipped && (testResult.numFailingTests > 0 || testResult.testExecError)) {
      this.printTestFileHeader(testResult.testFilePath, test.context.config, testResult);
      this.printTestFileFailureMessage(testResult.testFilePath, test.context.config, testResult);
    }
    this.forceFlushBufferedOutput();
  }
}
exports["default"] = AgentReporter;

/***/ },

/***/ "./src/BaseReporter.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
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

const {
  remove: preRunMessageRemove
} = _jestUtil().preRunMessage;
class BaseReporter {
  _error;
  log(message) {
    process.stderr.write(`${message}\n`);
  }
  onRunStart(_results, _options) {
    preRunMessageRemove(process.stderr);
  }

  /* eslint-disable @typescript-eslint/no-empty-function */
  onTestCaseResult(_test, _testCaseResult) {}
  onTestResult(_test, _testResult, _results) {}
  onTestStart(_test) {}
  onRunComplete(_testContexts, _aggregatedResults) {}
  /* eslint-enable */

  _setError(error) {
    this._error = error;
  }

  // Return an error that occurred during reporting. This error will
  // define whether the test run was successful or failed.
  getLastError() {
    return this._error;
  }
  __beginSynchronizedUpdate(write) {
    if (_jestUtil().isInteractive) {
      write('\u001B[?2026h');
    }
  }
  __endSynchronizedUpdate(write) {
    if (_jestUtil().isInteractive) {
      write('\u001B[?2026l');
    }
  }
}
exports["default"] = BaseReporter;

/***/ },

/***/ "./src/CoverageReporter.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function _v8Coverage() {
  const data = require("@bcoe/v8-coverage");
  _v8Coverage = function () {
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
function _glob() {
  const data = require("glob");
  _glob = function () {
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
function _istanbulLibCoverage() {
  const data = _interopRequireDefault(require("istanbul-lib-coverage"));
  _istanbulLibCoverage = function () {
    return data;
  };
  return data;
}
function _istanbulLibReport() {
  const data = _interopRequireDefault(require("istanbul-lib-report"));
  _istanbulLibReport = function () {
    return data;
  };
  return data;
}
function _istanbulLibSourceMaps() {
  const data = _interopRequireDefault(require("istanbul-lib-source-maps"));
  _istanbulLibSourceMaps = function () {
    return data;
  };
  return data;
}
function _istanbulReports() {
  const data = _interopRequireDefault(require("istanbul-reports"));
  _istanbulReports = function () {
    return data;
  };
  return data;
}
function _v8ToIstanbul() {
  const data = _interopRequireDefault(require("v8-to-istanbul"));
  _v8ToIstanbul = function () {
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
function _jestWorker() {
  const data = require("jest-worker");
  _jestWorker = function () {
    return data;
  };
  return data;
}
var _BaseReporter = _interopRequireDefault(__webpack_require__("./src/BaseReporter.ts"));
var _getWatermarks = _interopRequireDefault(__webpack_require__("./src/getWatermarks.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/* eslint-disable import-x/default */

/* eslint-enable import-x/default */

const FAIL_COLOR = _chalk().default.bold.red;
const RUNNING_TEST_COLOR = _chalk().default.bold.dim;
class CoverageReporter extends _BaseReporter.default {
  _context;
  _coverageMap;
  _globalConfig;
  _sourceMapStore;
  _v8CoverageResults;
  static filename = __filename;
  constructor(globalConfig, context) {
    super();
    this._context = context;
    this._coverageMap = _istanbulLibCoverage().default.createCoverageMap({});
    this._globalConfig = globalConfig;
    this._sourceMapStore = _istanbulLibSourceMaps().default.createSourceMapStore();
    this._v8CoverageResults = [];
  }
  onTestResult(_test, testResult) {
    if (testResult.v8Coverage) {
      this._v8CoverageResults.push(testResult.v8Coverage);
      return;
    }
    if (testResult.coverage) {
      this._coverageMap.merge(testResult.coverage);
    }
  }
  async onRunComplete(testContexts, aggregatedResults) {
    await this._addUntestedFiles(testContexts);
    const {
      map,
      reportContext
    } = await this._getCoverageResult();
    try {
      const coverageReporters = this._globalConfig.coverageReporters || [];
      if (!this._globalConfig.useStderr && coverageReporters.length === 0) {
        coverageReporters.push('text-summary');
      }
      for (let reporter of coverageReporters) {
        let additionalOptions = {};
        if (Array.isArray(reporter)) {
          [reporter, additionalOptions] = reporter;
        }
        _istanbulReports().default.create(reporter, {
          maxCols: process.stdout.columns || Number.POSITIVE_INFINITY,
          ...additionalOptions
        }).execute(reportContext);
      }
      aggregatedResults.coverageMap = map;
    } catch (error) {
      console.error(_chalk().default.red(`
        Failed to write coverage reports:
        ERROR: ${error.toString()}
        STACK: ${error.stack}
      `));
    }
    this._checkThreshold(map);
  }
  async _addUntestedFiles(testContexts) {
    const files = [];
    for (const context of testContexts) {
      const config = context.config;
      if (this._globalConfig.collectCoverageFrom && this._globalConfig.collectCoverageFrom.length > 0) {
        for (const filePath of context.hasteFS.matchFilesWithGlob(this._globalConfig.collectCoverageFrom, this._globalConfig.rootDir)) files.push({
          config,
          path: filePath
        });
      }
    }
    if (files.length === 0) {
      return;
    }
    if (_jestUtil().isInteractive) {
      process.stderr.write(RUNNING_TEST_COLOR('Running coverage on untested files...'));
    }
    let worker;
    if (this._globalConfig.maxWorkers <= 1) {
      worker = require('./CoverageWorker');
    } else {
      worker = new (_jestWorker().Worker)(require.resolve('./CoverageWorker'), {
        enableWorkerThreads: this._globalConfig.workerThreads,
        exposedMethods: ['worker'],
        forkOptions: {
          serialization: 'json'
        },
        maxRetries: 2,
        numWorkers: this._globalConfig.maxWorkers
      });
    }
    const instrumentation = files.map(async fileObj => {
      const filename = fileObj.path;
      const config = fileObj.config;
      const hasCoverageData = this._v8CoverageResults.some(v8Res => v8Res.some(innerRes => innerRes.result.url === filename));
      if (!hasCoverageData && !this._coverageMap.data[filename] && 'worker' in worker) {
        try {
          const result = await worker.worker({
            config,
            context: {
              changedFiles: this._context.changedFiles && [...this._context.changedFiles],
              sourcesRelatedToTestsInChangedFiles: this._context.sourcesRelatedToTestsInChangedFiles && [...this._context.sourcesRelatedToTestsInChangedFiles]
            },
            globalConfig: this._globalConfig,
            path: filename
          });
          if (result) {
            if (result.kind === 'V8Coverage') {
              this._v8CoverageResults.push([{
                codeTransformResult: undefined,
                result: result.result
              }]);
            } else {
              this._coverageMap.addFileCoverage(result.coverage);
            }
          }
        } catch (error) {
          console.error(_chalk().default.red([`Failed to collect coverage from ${filename}`, `ERROR: ${error.message}`, `STACK: ${error.stack}`].join('\n')));
        }
      }
    });
    try {
      await Promise.all(instrumentation);
    } catch {
      // Do nothing; errors were reported earlier to the console.
    }
    if (_jestUtil().isInteractive) {
      (0, _jestUtil().clearLine)(process.stderr);
    }
    if (worker && 'end' in worker && typeof worker.end === 'function') {
      await worker.end();
    }
  }
  _checkThreshold(map) {
    const {
      coverageThreshold
    } = this._globalConfig;
    if (coverageThreshold) {
      function check(name, thresholds, actuals) {
        return ['statements', 'branches', 'lines', 'functions'].reduce((errors, key) => {
          const actual = actuals[key].pct;
          const actualUncovered = actuals[key].total - actuals[key].covered;
          const threshold = thresholds[key];
          if (threshold !== undefined) {
            if (threshold < 0) {
              if (threshold * -1 < actualUncovered) {
                errors.push(`Jest: Uncovered count for ${key} (${actualUncovered}) ` + `exceeds ${name} threshold (${-1 * threshold})`);
              }
            } else if (actual < threshold) {
              errors.push(`Jest: Coverage for ${key} (${actual}%) does not meet "${name}" threshold (${threshold}%)`);
            }
          }
          return errors;
        }, []);
      }
      const THRESHOLD_GROUP_TYPES = {
        GLOB: 'glob',
        GLOBAL: 'global',
        PATH: 'path'
      };
      const coveredFiles = map.files();
      const thresholdGroups = Object.keys(coverageThreshold);
      const groupTypeByThresholdGroup = {};
      const filesByGlob = {};
      const coveredFilesSortedIntoThresholdGroup = coveredFiles.reduce((files, file) => {
        const pathOrGlobMatches = thresholdGroups.reduce((agg, thresholdGroup) => {
          // Skip 'global' here as it will be handled separately for all files
          if (thresholdGroup === THRESHOLD_GROUP_TYPES.GLOBAL) {
            return agg;
          }

          // Preserve trailing slash, but not required if root dir
          // See https://github.com/jestjs/jest/issues/12703
          const resolvedThresholdGroup = path().resolve(thresholdGroup);
          const suffix = (thresholdGroup.endsWith(path().sep) || process.platform === 'win32' && thresholdGroup.endsWith('/')) && !resolvedThresholdGroup.endsWith(path().sep) ? path().sep : '';
          const absoluteThresholdGroup = `${resolvedThresholdGroup}${suffix}`;

          // The threshold group might be a path:

          if (file.indexOf(absoluteThresholdGroup) === 0) {
            groupTypeByThresholdGroup[thresholdGroup] = THRESHOLD_GROUP_TYPES.PATH;
            agg.push([file, thresholdGroup]);
            return agg;
          }

          // If the threshold group is not a path it might be a glob:

          // Note: glob.sync is slow. By memoizing the files matching each glob
          // (rather than recalculating it for each covered file) we save a tonne
          // of execution time.
          if (filesByGlob[absoluteThresholdGroup] === undefined) {
            filesByGlob[absoluteThresholdGroup] = _glob().glob.sync(absoluteThresholdGroup, {
              windowsPathsNoEscape: true
            }).map(filePath => path().resolve(filePath));
          }
          if (filesByGlob[absoluteThresholdGroup].includes(file)) {
            groupTypeByThresholdGroup[thresholdGroup] = THRESHOLD_GROUP_TYPES.GLOB;
            agg.push([file, thresholdGroup]);
            return agg;
          }
          return agg;
        }, []);
        if (pathOrGlobMatches.length > 0) {
          files.push(...pathOrGlobMatches);
          return files;
        }

        // Neither a glob or a path? Toss it in global if there's a global threshold:
        if (thresholdGroups.includes(THRESHOLD_GROUP_TYPES.GLOBAL)) {
          groupTypeByThresholdGroup[THRESHOLD_GROUP_TYPES.GLOBAL] = THRESHOLD_GROUP_TYPES.GLOBAL;
          files.push([file, THRESHOLD_GROUP_TYPES.GLOBAL]);
          return files;
        }

        // A covered file that doesn't have a threshold:
        files.push([file, undefined]);
        return files;
      }, []);

      // Mark global threshold group if it exists
      if (thresholdGroups.includes(THRESHOLD_GROUP_TYPES.GLOBAL)) {
        groupTypeByThresholdGroup[THRESHOLD_GROUP_TYPES.GLOBAL] = THRESHOLD_GROUP_TYPES.GLOBAL;
      }
      const getFilesInThresholdGroup = thresholdGroup => coveredFilesSortedIntoThresholdGroup.filter(fileAndGroup => fileAndGroup[1] === thresholdGroup).map(fileAndGroup => fileAndGroup[0]);
      function combineCoverage(filePaths) {
        return filePaths.map(filePath => map.fileCoverageFor(filePath)).reduce((combinedCoverage, nextFileCoverage) => {
          if (combinedCoverage === undefined || combinedCoverage === null) {
            return nextFileCoverage.toSummary();
          }
          return combinedCoverage.merge(nextFileCoverage.toSummary());
        }, undefined);
      }
      let errors = [];
      for (const thresholdGroup of thresholdGroups) {
        switch (groupTypeByThresholdGroup[thresholdGroup]) {
          case THRESHOLD_GROUP_TYPES.GLOBAL:
            {
              const globalFiles = getFilesInThresholdGroup(THRESHOLD_GROUP_TYPES.GLOBAL);
              const coverage = combineCoverage(globalFiles.length > 0 ? globalFiles : coveredFiles);
              if (coverage) {
                errors = [...errors, ...check(thresholdGroup, coverageThreshold[thresholdGroup], coverage)];
              }
              break;
            }
          case THRESHOLD_GROUP_TYPES.PATH:
            {
              const coverage = combineCoverage(getFilesInThresholdGroup(thresholdGroup));
              if (coverage) {
                errors = [...errors, ...check(thresholdGroup, coverageThreshold[thresholdGroup], coverage)];
              }
              break;
            }
          case THRESHOLD_GROUP_TYPES.GLOB:
            for (const fileMatchingGlob of getFilesInThresholdGroup(thresholdGroup)) {
              errors = [...errors, ...check(fileMatchingGlob, coverageThreshold[thresholdGroup], map.fileCoverageFor(fileMatchingGlob).toSummary())];
            }
            break;
          default:
            // If the file specified by path is not found, error is returned.
            if (thresholdGroup !== THRESHOLD_GROUP_TYPES.GLOBAL) {
              errors = [...errors, `Jest: Coverage data for ${thresholdGroup} was not found.`];
            }
          // Sometimes all files in the coverage data are matched by
          // PATH and GLOB threshold groups in which case, don't error when
          // the global threshold group doesn't match any files.
        }
      }
      errors = errors.filter(err => err !== undefined && err !== null && err.length > 0);
      if (errors.length > 0) {
        this.log(`${FAIL_COLOR(errors.join('\n'))}`);
        this._setError(new Error(errors.join('\n')));
      }
    }
  }
  async _getCoverageResult() {
    if (this._globalConfig.coverageProvider === 'v8') {
      const mergedCoverages = (0, _v8Coverage().mergeProcessCovs)(this._v8CoverageResults.map(cov => ({
        result: cov.map(r => r.result)
      })));
      const fileTransforms = new Map();
      for (const res of this._v8CoverageResults) for (const r of res) {
        if (r.codeTransformResult && !fileTransforms.has(r.result.url)) {
          fileTransforms.set(r.result.url, r.codeTransformResult);
        }
      }
      const transformedCoverage = await Promise.all(mergedCoverages.result.map(async res => {
        const fileTransform = fileTransforms.get(res.url);
        let sourcemapContent = undefined;
        if (fileTransform?.sourceMapPath && fs().existsSync(fileTransform.sourceMapPath)) {
          sourcemapContent = JSON.parse(fs().readFileSync(fileTransform.sourceMapPath, 'utf8'));
        }
        const converter = (0, _v8ToIstanbul().default)(res.url, 0, fileTransform && sourcemapContent ? {
          originalSource: fileTransform.originalCode,
          source: fileTransform.code,
          sourceMap: {
            sourcemap: {
              file: res.url,
              ...sourcemapContent
            }
          }
        } : {
          source: fs().readFileSync(res.url, 'utf8')
        });
        await converter.load();
        converter.applyCoverage(res.functions);
        const istanbulData = converter.toIstanbul();
        return istanbulData;
      }));
      const map = _istanbulLibCoverage().default.createCoverageMap({});
      for (const res of transformedCoverage) map.merge(res);
      const reportContext = _istanbulLibReport().default.createContext({
        coverageMap: map,
        dir: this._globalConfig.coverageDirectory,
        watermarks: (0, _getWatermarks.default)(this._globalConfig)
      });
      return {
        map,
        reportContext
      };
    }
    const map = await this._sourceMapStore.transformCoverage(this._coverageMap);
    const reportContext = _istanbulLibReport().default.createContext({
      coverageMap: map,
      dir: this._globalConfig.coverageDirectory,
      sourceFinder: this._sourceMapStore.sourceFinder,
      watermarks: (0, _getWatermarks.default)(this._globalConfig)
    });
    return {
      map,
      reportContext
    };
  }
}
exports["default"] = CoverageReporter;

/***/ },

/***/ "./src/DefaultReporter.ts"
(__unused_webpack_module, exports, __webpack_require__) {



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
function _console() {
  const data = require("@jest/console");
  _console = function () {
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
var _BaseReporter = _interopRequireDefault(__webpack_require__("./src/BaseReporter.ts"));
var _Status = _interopRequireDefault(__webpack_require__("./src/Status.ts"));
var _getResultHeader = _interopRequireDefault(__webpack_require__("./src/getResultHeader.ts"));
var _getSnapshotStatus = _interopRequireDefault(__webpack_require__("./src/getSnapshotStatus.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const TITLE_BULLET = _chalk().default.bold('\u25CF ');
class DefaultReporter extends _BaseReporter.default {
  _clear; // ANSI clear sequence for the last printed status
  _err;
  _globalConfig;
  _out;
  _status;
  _bufferedOutput;
  static filename = __filename;
  constructor(globalConfig) {
    super();
    this._globalConfig = globalConfig;
    this._clear = '';
    this._out = process.stdout.write.bind(process.stdout);
    this._err = process.stderr.write.bind(process.stderr);
    this._status = new _Status.default(globalConfig);
    this._bufferedOutput = new Set();
    this.__wrapStdio(process.stdout);
    this.__wrapStdio(process.stderr);
    this._status.onChange(() => {
      this.__beginSynchronizedUpdate(this._globalConfig.useStderr ? this._err : this._out);
      this.__clearStatus();
      this.__printStatus();
      this.__endSynchronizedUpdate(this._globalConfig.useStderr ? this._err : this._out);
    });
  }
  __wrapStdio(stream) {
    const write = stream.write.bind(stream);
    let buffer = [];
    let timeout = null;
    const flushBufferedOutput = () => {
      const string = buffer.join('');
      buffer = [];

      // This is to avoid conflicts between random output and status text
      this.__beginSynchronizedUpdate(this._globalConfig.useStderr ? this._err : this._out);
      this.__clearStatus();
      if (string) {
        write(string);
      }
      this.__printStatus();
      this.__endSynchronizedUpdate(this._globalConfig.useStderr ? this._err : this._out);
      this._bufferedOutput.delete(flushBufferedOutput);
    };
    this._bufferedOutput.add(flushBufferedOutput);
    const debouncedFlush = () => {
      // If the process blows up no errors would be printed.
      // There should be a smart way to buffer stderr, but for now
      // we just won't buffer it.
      if (stream === process.stderr) {
        flushBufferedOutput();
      } else {
        if (!timeout) {
          timeout = setTimeout(() => {
            flushBufferedOutput();
            timeout = null;
          }, 100);
        }
      }
    };
    stream.write = chunk => {
      buffer.push(chunk);
      debouncedFlush();
      return true;
    };
  }

  // Don't wait for the debounced call and flush all output immediately.
  forceFlushBufferedOutput() {
    for (const flushBufferedOutput of this._bufferedOutput) {
      flushBufferedOutput();
    }
  }
  __clearStatus() {
    if (_jestUtil().isInteractive) {
      if (this._globalConfig.useStderr) {
        this._err(this._clear);
      } else {
        this._out(this._clear);
      }
    }
  }
  __printStatus() {
    const {
      content,
      clear
    } = this._status.get();
    this._clear = clear;
    if (_jestUtil().isInteractive) {
      if (this._globalConfig.useStderr) {
        this._err(content);
      } else {
        this._out(content);
      }
    }
  }
  onRunStart(aggregatedResults, options) {
    this._status.runStarted(aggregatedResults, options);
  }
  onTestStart(test) {
    this._status.testStarted(test.path, test.context.config);
  }
  onTestCaseResult(test, testCaseResult) {
    this._status.addTestCaseResult(test, testCaseResult);
  }
  onRunComplete() {
    this.forceFlushBufferedOutput();
    this._status.runFinished();
    process.stdout.write = this._out;
    process.stderr.write = this._err;
    (0, _jestUtil().clearLine)(process.stderr);
  }
  onTestResult(test, testResult, aggregatedResults) {
    this.testFinished(test.context.config, testResult, aggregatedResults);
    if (!testResult.skipped) {
      this.printTestFileHeader(testResult.testFilePath, test.context.config, testResult);
      this.printTestFileFailureMessage(testResult.testFilePath, test.context.config, testResult);
    }
    this.forceFlushBufferedOutput();
  }
  testFinished(config, testResult, aggregatedResults) {
    this._status.testFinished(config, testResult, aggregatedResults);
  }
  printTestFileHeader(testPath, config, result) {
    // log retry errors if any exist
    for (const testResult of result.testResults) {
      const testRetryReasons = testResult.retryReasons;
      if (testRetryReasons && testRetryReasons.length > 0) {
        this.log(`${_chalk().default.reset.inverse.bold.yellow(' LOGGING RETRY ERRORS ')} ${_chalk().default.bold(testResult.fullName)}`);
        for (const [index, retryReasons] of testRetryReasons.entries()) {
          let {
            message,
            stack
          } = (0, _jestMessageUtil().separateMessageFromStack)(retryReasons);
          stack = this._globalConfig.noStackTrace ? '' : _chalk().default.dim((0, _jestMessageUtil().formatStackTrace)(stack, config, this._globalConfig, testPath));
          message = (0, _jestMessageUtil().indentAllLines)(message);
          this.log(`${_chalk().default.reset.inverse.bold.blueBright(` RETRY ${index + 1} `)}\n`);
          this.log(`${message}\n${stack}\n`);
        }
      }
    }
    this.log((0, _getResultHeader.default)(result, this._globalConfig, config));
    if (result.console) {
      this.log(`  ${TITLE_BULLET}Console\n\n${(0, _console().getConsoleOutput)(result.console, config, this._globalConfig)}`);
    }
  }
  printTestFileFailureMessage(_testPath, _config, result) {
    if (result.failureMessage) {
      this.log(result.failureMessage);
    }
    const didUpdate = this._globalConfig.updateSnapshot === 'all';
    const snapshotStatuses = (0, _getSnapshotStatus.default)(result.snapshot, didUpdate);
    for (const status of snapshotStatuses) this.log(status);
  }
}
exports["default"] = DefaultReporter;

/***/ },

/***/ "./src/GitHubActionsReporter.ts"
(__unused_webpack_module, exports, __webpack_require__) {



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
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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
var _BaseReporter = _interopRequireDefault(__webpack_require__("./src/BaseReporter.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const titleSeparator = ' \u203A ';
const ICONS = _jestUtil().specialChars.ICONS;
class GitHubActionsReporter extends _BaseReporter.default {
  static filename = __filename;
  options;
  globalConfig;
  constructor(globalConfig, reporterOptions = {}) {
    super();
    this.globalConfig = globalConfig;
    this.options = {
      silent: typeof reporterOptions.silent === 'boolean' ? reporterOptions.silent : true
    };
  }
  onTestResult(test, testResult, aggregatedResults) {
    this.generateAnnotations(test, testResult);
    if (!this.options.silent) {
      this.printFullResult(test.context, testResult);
    }
    if (this.isLastTestSuite(aggregatedResults)) {
      this.printFailedTestLogs(test, testResult.console, aggregatedResults);
    }
  }
  generateAnnotations({
    context
  }, {
    testResults
  }) {
    for (const result of testResults) {
      const title = [...result.ancestorTitles, result.title].join(titleSeparator);
      if (result.retryReasons) for (const [index, retryReason] of result.retryReasons.entries()) {
        this.#createAnnotation({
          ...this.#getMessageDetails(retryReason, context.config),
          title: `RETRY ${index + 1}: ${title}`,
          type: 'warning'
        });
      }
      for (const failureMessage of result.failureMessages) {
        this.#createAnnotation({
          ...this.#getMessageDetails(failureMessage, context.config),
          title,
          type: 'error'
        });
      }
    }
  }
  #getMessageDetails(failureMessage, config) {
    const {
      message,
      stack
    } = (0, _jestMessageUtil().separateMessageFromStack)(failureMessage);
    const stackLines = (0, _jestMessageUtil().getStackTraceLines)(stack);
    const topFrame = (0, _jestMessageUtil().getTopFrame)(stackLines);
    const normalizedStackLines = stackLines.map(line => (0, _jestMessageUtil().formatPath)(line, config));
    const messageText = [message, ...normalizedStackLines].join('\n');
    return {
      file: topFrame?.file,
      line: topFrame?.line,
      message: messageText
    };
  }
  #createAnnotation({
    file,
    line,
    message,
    title,
    type
  }) {
    message = (0, _nodeUtil().stripVTControlCharacters)(
    // copied from: https://github.com/actions/toolkit/blob/main/packages/core/src/command.ts
    message.replaceAll('%', '%25').replaceAll('\r', '%0D').replaceAll('\n', '%0A'));
    this.log(`\n::${type} file=${file},line=${line},title=${title}::${message}`);
  }
  isLastTestSuite(results) {
    const passedTestSuites = results.numPassedTestSuites;
    const failedTestSuites = results.numFailedTestSuites;
    const totalTestSuites = results.numTotalTestSuites;
    const computedTotal = passedTestSuites + failedTestSuites;
    if (computedTotal < totalTestSuites) {
      return false;
    } else if (computedTotal === totalTestSuites) {
      return true;
    } else {
      throw new Error(`Sum(${computedTotal}) of passed (${passedTestSuites}) and failed (${failedTestSuites}) test suites is greater than the total number of test suites (${totalTestSuites}). Please report the bug at https://github.com/jestjs/jest/issues`);
    }
  }
  printFullResult(context, results) {
    const rootDir = context.config.rootDir;
    let testDir = results.testFilePath.replace(rootDir, '');
    testDir = testDir.slice(1);
    const resultTree = this.getResultTree(results.testResults, testDir, results.perfStats);
    this.printResultTree(resultTree, context.config, results.console);
  }
  arrayEqual(a1, a2) {
    if (a1.length !== a2.length) {
      return false;
    }
    for (const [index, element] of a1.entries()) {
      if (element !== a2[index]) {
        return false;
      }
    }
    return true;
  }
  arrayChild(a1, a2) {
    if (a1.length - a2.length !== 1) {
      return false;
    }
    for (const [index, element] of a2.entries()) {
      if (element !== a1[index]) {
        return false;
      }
    }
    return true;
  }
  getResultTree(suiteResult, testPath, suitePerf) {
    const root = {
      children: [],
      name: testPath,
      passed: true,
      performanceInfo: suitePerf
    };
    const branches = [];
    for (const element of suiteResult) {
      if (element.ancestorTitles.length === 0) {
        if (element.status === 'failed') {
          root.passed = false;
        }
        const duration = element.duration || 1;
        root.children.push({
          children: [],
          duration,
          name: element.title,
          status: element.status
        });
      } else {
        let alreadyInserted = false;
        for (const branch of branches) {
          if (this.arrayEqual(branch, element.ancestorTitles.slice(0, 1))) {
            alreadyInserted = true;
            break;
          }
        }
        if (!alreadyInserted) {
          branches.push(element.ancestorTitles.slice(0, 1));
        }
      }
    }
    for (const element of branches) {
      const newChild = this.getResultChildren(suiteResult, element);
      if (!newChild.passed) {
        root.passed = false;
      }
      root.children.push(newChild);
    }
    return root;
  }
  getResultChildren(suiteResult, ancestors) {
    const node = {
      children: [],
      name: ancestors.at(-1),
      passed: true
    };
    const branches = [];
    for (const element of suiteResult) {
      let duration = element.duration;
      if (!duration || Number.isNaN(duration)) {
        duration = 1;
      }
      if (this.arrayEqual(element.ancestorTitles, ancestors)) {
        if (element.status === 'failed') {
          node.passed = false;
        }
        node.children.push({
          children: [],
          duration,
          name: element.title,
          status: element.status
        });
      } else if (this.arrayChild(element.ancestorTitles.slice(0, ancestors.length + 1), ancestors)) {
        let alreadyInserted = false;
        for (const branch of branches) {
          if (this.arrayEqual(branch, element.ancestorTitles.slice(0, ancestors.length + 1))) {
            alreadyInserted = true;
            break;
          }
        }
        if (!alreadyInserted) {
          branches.push(element.ancestorTitles.slice(0, ancestors.length + 1));
        }
      }
    }
    for (const element of branches) {
      const newChild = this.getResultChildren(suiteResult, element);
      if (!newChild.passed) {
        node.passed = false;
      }
      node.children.push(newChild);
    }
    return node;
  }
  printResultTree(resultTree, config, consoleLog) {
    let perfMs;
    if (resultTree.performanceInfo.slow) {
      perfMs = ` (${_chalk().default.red.inverse(`${resultTree.performanceInfo.runtime} ms`)})`;
    } else {
      perfMs = ` (${resultTree.performanceInfo.runtime} ms)`;
    }
    if (resultTree.passed) {
      this.startGroup(`${_chalk().default.bold.green.inverse('PASS')} ${resultTree.name}${perfMs}`);
      if (consoleLog && !this.options.silent) {
        this.log((0, _console().getConsoleOutput)(consoleLog, config, this.globalConfig));
      }
      for (const child of resultTree.children) {
        this.recursivePrintResultTree(child, true, 1);
      }
      this.endGroup();
    } else {
      this.log(`  ${_chalk().default.bold.red.inverse('FAIL')} ${resultTree.name}${perfMs}`);
      for (const child of resultTree.children) {
        this.recursivePrintResultTree(child, false, 1);
      }
    }
  }
  recursivePrintResultTree(resultTree, alreadyGrouped, depth) {
    if (resultTree.children.length === 0) {
      if (!('duration' in resultTree)) {
        throw new Error('Expected a leaf. Got a node.');
      }
      let numberSpaces = depth;
      if (!alreadyGrouped) {
        numberSpaces++;
      }
      const spaces = '  '.repeat(numberSpaces);
      let resultSymbol;
      switch (resultTree.status) {
        case 'passed':
          resultSymbol = _chalk().default.green(ICONS.success);
          break;
        case 'failed':
          resultSymbol = _chalk().default.red(ICONS.failed);
          break;
        case 'todo':
          resultSymbol = _chalk().default.magenta(ICONS.todo);
          break;
        case 'pending':
        case 'skipped':
          resultSymbol = _chalk().default.yellow(ICONS.pending);
          break;
      }
      this.log(`${spaces + resultSymbol} ${resultTree.name} (${resultTree.duration} ms)`);
    } else {
      if (!('passed' in resultTree)) {
        throw new Error('Expected a node. Got a leaf');
      }
      if (resultTree.passed) {
        if (alreadyGrouped) {
          this.log('  '.repeat(depth) + resultTree.name);
          for (const child of resultTree.children) {
            this.recursivePrintResultTree(child, true, depth + 1);
          }
        } else {
          this.startGroup('  '.repeat(depth) + resultTree.name);
          for (const child of resultTree.children) {
            this.recursivePrintResultTree(child, true, depth + 1);
          }
          this.endGroup();
        }
      } else {
        this.log('  '.repeat(depth + 1) + resultTree.name);
        for (const child of resultTree.children) {
          this.recursivePrintResultTree(child, false, depth + 1);
        }
      }
    }
  }
  printFailedTestLogs(context, consoleLog, testResults) {
    const rootDir = context.context.config.rootDir;
    const results = testResults.testResults;
    let written = false;
    for (const result of results) {
      let testDir = result.testFilePath;
      testDir = testDir.replace(rootDir, '');
      testDir = testDir.slice(1);
      if (result.failureMessage) {
        if (!written) {
          this.log('');
          written = true;
        }
        this.startGroup(`Errors thrown in ${testDir}`);
        if (consoleLog && !this.options.silent) {
          this.log((0, _console().getConsoleOutput)(consoleLog, context.context.config, this.globalConfig));
        }
        this.log(result.failureMessage);
        this.endGroup();
      }
    }
    return written;
  }
  startGroup(title) {
    this.log(`::group::${title}`);
  }
  endGroup() {
    this.log('::endgroup::');
  }
}
exports["default"] = GitHubActionsReporter;

/***/ },

/***/ "./src/NotifyReporter.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function path() {
  const data = _interopRequireWildcard(require("node:path"));
  path = function () {
    return data;
  };
  return data;
}
function util() {
  const data = _interopRequireWildcard(require("node:util"));
  util = function () {
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
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
var _BaseReporter = _interopRequireDefault(__webpack_require__("./src/BaseReporter.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const isDarwin = process.platform === 'darwin';
const icon = path().resolve(__dirname, '../assets/jest_logo.png');
class NotifyReporter extends _BaseReporter.default {
  _notifier = loadNotifier();
  _globalConfig;
  _context;
  static filename = __filename;
  constructor(globalConfig, context) {
    super();
    this._globalConfig = globalConfig;
    this._context = context;
  }
  onRunComplete(testContexts, result) {
    const success = result.numFailedTests === 0 && result.numRuntimeErrorTestSuites === 0;
    const firstContext = testContexts.values().next();
    const hasteFS = firstContext && firstContext.value && firstContext.value.hasteFS;
    let packageName;
    if (hasteFS == null) {
      packageName = this._globalConfig.rootDir;
    } else {
      // assuming root package.json is the first one
      const [filePath] = hasteFS.matchFiles('package.json');
      packageName = filePath == null ? this._globalConfig.rootDir : hasteFS.getModuleName(filePath);
    }
    packageName = packageName == null ? '' : `${packageName} - `;
    const notifyMode = this._globalConfig.notifyMode;
    const statusChanged = this._context.previousSuccess !== success || this._context.firstRun;
    const testsHaveRun = result.numTotalTests !== 0;
    if (testsHaveRun && success && (notifyMode === 'always' || notifyMode === 'success' || notifyMode === 'success-change' || notifyMode === 'change' && statusChanged || notifyMode === 'failure-change' && statusChanged)) {
      const title = util().format('%s%d%% Passed', packageName, 100);
      const message = `${isDarwin ? '\u2705 ' : ''}${(0, _jestUtil().pluralize)('test', result.numPassedTests)} passed`;
      this._notifier.notify({
        hint: 'int:transient:1',
        icon,
        message,
        timeout: false,
        title
      });
    } else if (testsHaveRun && !success && (notifyMode === 'always' || notifyMode === 'failure' || notifyMode === 'failure-change' || notifyMode === 'change' && statusChanged || notifyMode === 'success-change' && statusChanged)) {
      const failed = result.numFailedTests / result.numTotalTests;
      const title = util().format('%s%d%% Failed', packageName, Math.ceil(Number.isNaN(failed) ? 0 : failed * 100));
      const message = util().format(`${isDarwin ? '\u26D4\uFE0F ' : ''}%d of %d tests failed`, result.numFailedTests, result.numTotalTests);
      const watchMode = this._globalConfig.watch || this._globalConfig.watchAll;
      const restartAnswer = 'Run again';
      const quitAnswer = 'Exit tests';
      if (watchMode) {
        this._notifier.notify({
          // @ts-expect-error - not all options are supported by all systems (specifically `actions` and `hint`)
          actions: [restartAnswer, quitAnswer],
          closeLabel: 'Close',
          hint: 'int:transient:1',
          icon,
          message,
          timeout: false,
          title
        }, (err, _, metadata) => {
          if (err || !metadata) {
            return;
          }
          if (metadata.activationValue === quitAnswer) {
            (0, _exitX().default)(0);
            return;
          }
          if (metadata.activationValue === restartAnswer && this._context.startRun) {
            this._context.startRun(this._globalConfig);
          }
        });
      } else {
        this._notifier.notify({
          hint: 'int:transient:1',
          icon,
          message,
          timeout: false,
          title
        });
      }
    }
    this._context.previousSuccess = success;
    this._context.firstRun = false;
  }
}
exports["default"] = NotifyReporter;
function loadNotifier() {
  try {
    return require('node-notifier');
  } catch (error) {
    if (error.code !== 'MODULE_NOT_FOUND') {
      throw error;
    }
    throw new Error('notify reporter requires optional peer dependency "node-notifier" but it was not found');
  }
}

/***/ },

/***/ "./src/Status.ts"
(__unused_webpack_module, exports, __webpack_require__) {



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
function _stringLength() {
  const data = _interopRequireDefault(require("string-length"));
  _stringLength = function () {
    return data;
  };
  return data;
}
var _getSummary = _interopRequireDefault(__webpack_require__("./src/getSummary.ts"));
var _printDisplayName = _interopRequireDefault(__webpack_require__("./src/printDisplayName.ts"));
var _trimAndFormatPath = _interopRequireDefault(__webpack_require__("./src/trimAndFormatPath.ts"));
var _wrapAnsiString = _interopRequireDefault(__webpack_require__("./src/wrapAnsiString.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const RUNNING_TEXT = ' RUNS ';
const RUNNING = `${_chalk().default.reset.inverse.yellow.bold(RUNNING_TEXT)} `;

/**
 * This class is a perf optimization for sorting the list of currently
 * running tests. It tries to keep tests in the same positions without
 * shifting the whole list.
 */
class CurrentTestList {
  _array;
  constructor() {
    this._array = [];
  }
  add(testPath, config) {
    const index = this._array.indexOf(null);
    const record = {
      config,
      testPath
    };
    if (index === -1) {
      this._array.push(record);
    } else {
      this._array[index] = record;
    }
  }
  delete(testPath) {
    const record = this._array.find(record => record !== null && record.testPath === testPath);
    this._array[this._array.indexOf(record || null)] = null;
  }
  get() {
    return this._array;
  }
}
/**
 * A class that generates the CLI status of currently running tests
 * and also provides an ANSI escape sequence to remove status lines
 * from the terminal.
 */
class Status {
  _cache = null;
  _callback;
  _currentTests;
  _currentTestCases;
  _done;
  _emitScheduled;
  _estimatedTime;
  _interval;
  _aggregatedResults;
  _showStatus;
  constructor(_globalConfig) {
    this._globalConfig = _globalConfig;
    this._currentTests = new CurrentTestList();
    this._currentTestCases = [];
    this._done = false;
    this._emitScheduled = false;
    this._estimatedTime = 0;
    this._showStatus = false;
  }
  onChange(callback) {
    this._callback = callback;
  }
  runStarted(aggregatedResults, options) {
    this._estimatedTime = options && options.estimatedTime || 0;
    this._showStatus = options && options.showStatus;
    this._interval = setInterval(() => this._tick(), 1000);
    this._aggregatedResults = aggregatedResults;
    this._debouncedEmit();
  }
  runFinished() {
    this._done = true;
    if (this._interval) clearInterval(this._interval);
    this._emit();
  }
  addTestCaseResult(test, testCaseResult) {
    this._currentTestCases.push({
      test,
      testCaseResult
    });
    if (this._showStatus) {
      this._debouncedEmit();
    } else {
      this._emit();
    }
  }
  testStarted(testPath, config) {
    this._currentTests.add(testPath, config);
    if (this._showStatus) {
      this._debouncedEmit();
    } else {
      this._emit();
    }
  }
  testFinished(_config, testResult, aggregatedResults) {
    const {
      testFilePath
    } = testResult;
    this._aggregatedResults = aggregatedResults;
    this._currentTests.delete(testFilePath);
    this._currentTestCases = this._currentTestCases.filter(({
      test
    }) => {
      if (_config !== test.context.config) {
        return true;
      }
      return test.path !== testFilePath;
    });
    this._debouncedEmit();
  }
  get() {
    if (this._cache) {
      return this._cache;
    }
    if (this._done) {
      return {
        clear: '',
        content: ''
      };
    }
    const width = process.stdout.columns;
    let content = '\n';
    for (const record of this._currentTests.get()) {
      if (record) {
        const {
          config,
          testPath
        } = record;
        const projectDisplayName = config.displayName ? `${(0, _printDisplayName.default)(config)} ` : '';
        const prefix = RUNNING + projectDisplayName;
        content += `${(0, _wrapAnsiString.default)(prefix + (0, _trimAndFormatPath.default)((0, _stringLength().default)(prefix), config, testPath, width), width)}\n`;
      }
    }
    if (this._showStatus && this._aggregatedResults) {
      content += `\n${(0, _getSummary.default)(this._aggregatedResults, {
        currentTestCases: this._currentTestCases,
        estimatedTime: this._estimatedTime,
        roundTime: true,
        seed: this._globalConfig.seed,
        showSeed: this._globalConfig.showSeed,
        width
      })}`;
    }
    let height = 0;
    for (const char of content) {
      if (char === '\n') {
        height++;
      }
    }
    const clear = '\r\u001B[K\r\u001B[1A'.repeat(height);
    return this._cache = {
      clear,
      content
    };
  }
  _emit() {
    this._cache = null;
    if (this._callback) this._callback();
  }
  _debouncedEmit() {
    if (!this._emitScheduled) {
      // Perf optimization to avoid two separate renders When
      // one test finishes and another test starts executing.
      this._emitScheduled = true;
      setTimeout(() => {
        this._emit();
        this._emitScheduled = false;
      }, 100);
    }
  }
  _tick() {
    this._debouncedEmit();
  }
}
exports["default"] = Status;

/***/ },

/***/ "./src/SummaryReporter.ts"
(__unused_webpack_module, exports, __webpack_require__) {



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
var _BaseReporter = _interopRequireDefault(__webpack_require__("./src/BaseReporter.ts"));
var _getResultHeader = _interopRequireDefault(__webpack_require__("./src/getResultHeader.ts"));
var _getSnapshotSummary = _interopRequireDefault(__webpack_require__("./src/getSnapshotSummary.ts"));
var _getSummary = _interopRequireDefault(__webpack_require__("./src/getSummary.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const NPM_EVENTS = new Set(['prepublish', 'publish', 'postpublish', 'preinstall', 'install', 'postinstall', 'preuninstall', 'uninstall', 'postuninstall', 'preversion', 'version', 'postversion', 'pretest', 'test', 'posttest', 'prestop', 'stop', 'poststop', 'prestart', 'start', 'poststart', 'prerestart', 'restart', 'postrestart']);
const {
  npm_config_user_agent,
  npm_lifecycle_event,
  npm_lifecycle_script
} = process.env;
class SummaryReporter extends _BaseReporter.default {
  _estimatedTime;
  _globalConfig;
  _summaryThreshold;
  static filename = __filename;
  constructor(globalConfig, options) {
    super();
    this._globalConfig = globalConfig;
    this._estimatedTime = 0;
    this._validateOptions(options);
    this._summaryThreshold = options?.summaryThreshold ?? 20;
  }
  _validateOptions(options) {
    if (options?.summaryThreshold && typeof options.summaryThreshold !== 'number') {
      throw new TypeError('The option summaryThreshold should be a number');
    }
  }

  // If we write more than one character at a time it is possible that
  // Node.js exits in the middle of printing the result. This was first observed
  // in Node.js 0.10 and still persists in Node.js 6.7+.
  // Let's print the test failure summary character by character which is safer
  // when hundreds of tests are failing.
  _write(string) {
    for (let i = 0; i < string.length; i++) {
      process.stderr.write(string.charAt(i));
    }
  }
  onRunStart(aggregatedResults, options) {
    super.onRunStart(aggregatedResults, options);
    this._estimatedTime = options.estimatedTime;
  }
  onRunComplete(testContexts, aggregatedResults) {
    const {
      numTotalTestSuites,
      testResults,
      wasInterrupted
    } = aggregatedResults;
    if (numTotalTestSuites) {
      const lastResult = testResults.at(-1);
      // Print a newline if the last test did not fail to line up newlines
      // similar to when an error would have been thrown in the test.
      if (!this._globalConfig.verbose && lastResult && !lastResult.numFailingTests && !lastResult.testExecError) {
        this.log('');
      }
      this._printSummary(aggregatedResults, this._globalConfig);
      this._printSnapshotSummary(aggregatedResults.snapshot, this._globalConfig);
      let message = (0, _getSummary.default)(aggregatedResults, {
        estimatedTime: this._estimatedTime,
        seed: this._globalConfig.seed,
        showSeed: this._globalConfig.showSeed
      });
      if (!this._globalConfig.silent) {
        message += `\n${wasInterrupted ? _chalk().default.bold.red('Test run was interrupted.') : this._getTestSummary(testContexts, this._globalConfig)}`;
      }
      this.log(message);
    }
  }
  _printSnapshotSummary(snapshots, globalConfig) {
    if (snapshots.added || snapshots.filesRemoved || snapshots.unchecked || snapshots.unmatched || snapshots.updated) {
      let updateCommand;
      const event = npm_lifecycle_event || '';
      const prefix = NPM_EVENTS.has(event) ? '' : 'run ';
      const isYarn = typeof npm_config_user_agent === 'string' && npm_config_user_agent.includes('yarn');
      const client = isYarn ? 'yarn' : 'npm';
      const scriptUsesJest = typeof npm_lifecycle_script === 'string' && npm_lifecycle_script.includes('jest');
      if (globalConfig.watch || globalConfig.watchAll) {
        updateCommand = 'press `u`';
      } else if (event && scriptUsesJest) {
        updateCommand = `run \`${`${client} ${prefix}${event}${isYarn ? '' : ' --'}`} -u\``;
      } else {
        updateCommand = 're-run jest with `-u`';
      }
      const snapshotSummary = (0, _getSnapshotSummary.default)(snapshots, globalConfig, updateCommand);
      for (const summary of snapshotSummary) this.log(summary);
      this.log(''); // print empty line
    }
  }
  _printSummary(aggregatedResults, globalConfig) {
    // If there were any failing tests and there was a large number of tests
    // executed, re-print the failing results at the end of execution output.
    const failedTests = aggregatedResults.numFailedTests;
    const runtimeErrors = aggregatedResults.numRuntimeErrorTestSuites;
    if (failedTests + runtimeErrors > 0 && aggregatedResults.numTotalTestSuites > this._summaryThreshold) {
      this.log(_chalk().default.bold('Summary of all failing tests'));
      for (const testResult of aggregatedResults.testResults) {
        const {
          failureMessage
        } = testResult;
        if (failureMessage) {
          this._write(`${(0, _getResultHeader.default)(testResult, globalConfig)}\n${failureMessage}\n`);
        }
      }
      this.log(''); // print empty line
    }
  }
  _getTestSummary(testContexts, globalConfig) {
    const testPathPatterns = globalConfig.testPathPatterns;
    const getMatchingTestsInfo = () => {
      const prefix = globalConfig.findRelatedTests ? ' related to files matching ' : ' matching ';
      return _chalk().default.dim(prefix) + testPathPatterns.toPretty();
    };
    let testInfo = '';
    if (globalConfig.runTestsByPath) {
      testInfo = _chalk().default.dim(' within paths');
    } else if (globalConfig.onlyChanged) {
      testInfo = _chalk().default.dim(' related to changed files');
    } else if (testPathPatterns.isSet()) {
      testInfo = getMatchingTestsInfo();
    }
    let nameInfo = '';
    if (globalConfig.runTestsByPath) {
      nameInfo = ` ${globalConfig.nonFlagArgs.map(p => `"${p}"`).join(', ')}`;
    } else if (globalConfig.testNamePattern) {
      nameInfo = `${_chalk().default.dim(' with tests matching ')}"${globalConfig.testNamePattern}"`;
    }
    const contextInfo = testContexts.size > 1 ? _chalk().default.dim(' in ') + testContexts.size + _chalk().default.dim(' projects') : '';
    return _chalk().default.dim('Ran all test suites') + testInfo + nameInfo + contextInfo + _chalk().default.dim('.');
  }
}
exports["default"] = SummaryReporter;

/***/ },

/***/ "./src/VerboseReporter.ts"
(__unused_webpack_module, exports, __webpack_require__) {



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
var _DefaultReporter = _interopRequireDefault(__webpack_require__("./src/DefaultReporter.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const {
  ICONS
} = _jestUtil().specialChars;
class VerboseReporter extends _DefaultReporter.default {
  _globalConfig;
  static filename = __filename;
  constructor(globalConfig) {
    super(globalConfig);
    this._globalConfig = globalConfig;
  }

  // Verbose mode is for debugging. Buffering of output is undesirable.
  // See https://github.com/jestjs/jest/issues/8208
  __wrapStdio(stream) {
    const write = stream.write.bind(stream);
    stream.write = chunk => {
      this.__clearStatus();
      write(chunk);
      this.__printStatus();
      return true;
    };
  }
  static filterTestResults(testResults) {
    return testResults.filter(({
      status
    }) => status !== 'pending');
  }
  static groupTestsBySuites(testResults) {
    const root = {
      suites: [],
      tests: [],
      title: ''
    };
    for (const testResult of testResults) {
      let targetSuite = root;

      // Find the target suite for this test,
      // creating nested suites as necessary.
      for (const title of testResult.ancestorTitles) {
        let matchingSuite = targetSuite.suites.find(s => s.title === title);
        if (!matchingSuite) {
          matchingSuite = {
            suites: [],
            tests: [],
            title
          };
          targetSuite.suites.push(matchingSuite);
        }
        targetSuite = matchingSuite;
      }
      targetSuite.tests.push(testResult);
    }
    return root;
  }
  onTestResult(test, result, aggregatedResults) {
    super.testFinished(test.context.config, result, aggregatedResults);
    if (!result.skipped) {
      this.printTestFileHeader(result.testFilePath, test.context.config, result);
      const verbose = test.context.config.verbose ?? this._globalConfig.verbose;
      if (verbose && !result.testExecError && !result.skipped) {
        this._logTestResults(result.testResults);
      }
      this.printTestFileFailureMessage(result.testFilePath, test.context.config, result);
    }
    super.forceFlushBufferedOutput();
  }
  _logTestResults(testResults) {
    this._logSuite(VerboseReporter.groupTestsBySuites(testResults), 0);
    this._logLine();
  }
  _logSuite(suite, indentLevel) {
    if (suite.title) {
      this._logLine(suite.title, indentLevel);
    }
    this._logTests(suite.tests, indentLevel + 1);
    for (const innerSuite of suite.suites) {
      this._logSuite(innerSuite, indentLevel + 1);
    }
  }
  _getIcon(status) {
    if (status === 'failed') {
      return _chalk().default.red(ICONS.failed);
    } else if (status === 'pending') {
      return _chalk().default.yellow(ICONS.pending);
    } else if (status === 'todo') {
      return _chalk().default.magenta(ICONS.todo);
    } else {
      return _chalk().default.green(ICONS.success);
    }
  }
  _logTest(test, indentLevel) {
    const status = this._getIcon(test.status);
    const time = test.duration ? ` (${(0, _jestUtil().formatTime)(Math.round(test.duration))})` : '';
    this._logLine(`${status} ${_chalk().default.dim(test.title + time)}`, indentLevel);
  }
  _logTests(tests, indentLevel) {
    if (this._globalConfig.expand) {
      for (const test of tests) this._logTest(test, indentLevel);
    } else {
      const summedTests = tests.reduce((result, test) => {
        if (test.status === 'pending') {
          result.pending.push(test);
        } else if (test.status === 'todo') {
          result.todo.push(test);
        } else {
          this._logTest(test, indentLevel);
        }
        return result;
      }, {
        pending: [],
        todo: []
      });
      const logTodoOrPendingTest = this._logTodoOrPendingTest(indentLevel);
      if (summedTests.pending.length > 0) {
        for (const test of summedTests.pending) logTodoOrPendingTest(test);
      }
      if (summedTests.todo.length > 0) {
        for (const test of summedTests.todo) logTodoOrPendingTest(test);
      }
    }
  }
  _logTodoOrPendingTest(indentLevel) {
    return test => {
      const printedTestStatus = test.status === 'pending' ? 'skipped' : test.status;
      const icon = this._getIcon(test.status);
      const text = _chalk().default.dim(`${printedTestStatus} ${test.title}`);
      this._logLine(`${icon} ${text}`, indentLevel);
    };
  }
  _logLine(str, indentLevel) {
    const indentation = '  '.repeat(indentLevel || 0);
    this.log(indentation + (str || ''));
  }
}
exports["default"] = VerboseReporter;

/***/ },

/***/ "./src/formatTestPath.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = formatTestPath;
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
function _slash() {
  const data = _interopRequireDefault(require("slash"));
  _slash = function () {
    return data;
  };
  return data;
}
var _relativePath = _interopRequireDefault(__webpack_require__("./src/relativePath.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function formatTestPath(config, testPath) {
  const {
    dirname,
    basename
  } = (0, _relativePath.default)(config, testPath);
  return (0, _slash().default)(_chalk().default.dim(dirname + path().sep) + _chalk().default.bold(basename));
}

/***/ },

/***/ "./src/getResultHeader.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getResultHeader;
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
var _formatTestPath = _interopRequireDefault(__webpack_require__("./src/formatTestPath.ts"));
var _printDisplayName = _interopRequireDefault(__webpack_require__("./src/printDisplayName.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const LONG_TEST_COLOR = _chalk().default.reset.bold.bgRed;
// Explicitly reset for these messages since they can get written out in the
// middle of error logging
const FAIL_TEXT = 'FAIL';
const PASS_TEXT = 'PASS';
const FAIL = _chalk().default.supportsColor ? _chalk().default.reset.inverse.bold.red(` ${FAIL_TEXT} `) : FAIL_TEXT;
const PASS = _chalk().default.supportsColor ? _chalk().default.reset.inverse.bold.green(` ${PASS_TEXT} `) : PASS_TEXT;
function getResultHeader(result, globalConfig, projectConfig) {
  const testPath = result.testFilePath;
  const status = result.numFailingTests > 0 || result.testExecError ? FAIL : PASS;
  const testDetail = [];
  if (result.perfStats?.slow) {
    const runTime = result.perfStats.runtime / 1000;
    testDetail.push(LONG_TEST_COLOR((0, _jestUtil().formatTime)(runTime, 0)));
  }
  if (result.memoryUsage) {
    const toMB = bytes => Math.floor(bytes / 1024 / 1024);
    testDetail.push(`${toMB(result.memoryUsage)} MB heap size`);
  }
  const projectDisplayName = projectConfig && projectConfig.displayName ? `${(0, _printDisplayName.default)(projectConfig)} ` : '';
  return `${status} ${projectDisplayName}${(0, _formatTestPath.default)(projectConfig ?? globalConfig, testPath)}${testDetail.length > 0 ? ` (${testDetail.join(', ')})` : ''}`;
}

/***/ },

/***/ "./src/getSnapshotStatus.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getSnapshotStatus;
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

const ARROW = ' \u203A ';
const DOT = ' \u2022 ';
const FAIL_COLOR = _chalk().default.bold.red;
const SNAPSHOT_ADDED = _chalk().default.bold.green;
const SNAPSHOT_UPDATED = _chalk().default.bold.green;
const SNAPSHOT_OUTDATED = _chalk().default.bold.yellow;
function getSnapshotStatus(snapshot, afterUpdate) {
  const statuses = [];
  if (snapshot.added) {
    statuses.push(SNAPSHOT_ADDED(`${ARROW + (0, _jestUtil().pluralize)('snapshot', snapshot.added)} written.`));
  }
  if (snapshot.updated) {
    statuses.push(SNAPSHOT_UPDATED(`${ARROW + (0, _jestUtil().pluralize)('snapshot', snapshot.updated)} updated.`));
  }
  if (snapshot.unmatched) {
    statuses.push(FAIL_COLOR(`${ARROW + (0, _jestUtil().pluralize)('snapshot', snapshot.unmatched)} failed.`));
  }
  if (snapshot.unchecked) {
    if (afterUpdate) {
      statuses.push(SNAPSHOT_UPDATED(`${ARROW + (0, _jestUtil().pluralize)('snapshot', snapshot.unchecked)} removed.`));
    } else {
      statuses.push(`${SNAPSHOT_OUTDATED(`${ARROW + (0, _jestUtil().pluralize)('snapshot', snapshot.unchecked)} obsolete`)}.`);
    }
    for (const key of snapshot.uncheckedKeys) {
      statuses.push(`  ${DOT}${key}`);
    }
  }
  if (snapshot.fileDeleted) {
    statuses.push(SNAPSHOT_UPDATED(`${ARROW}snapshot file removed.`));
  }
  return statuses;
}

/***/ },

/***/ "./src/getSnapshotSummary.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getSnapshotSummary;
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
var _formatTestPath = _interopRequireDefault(__webpack_require__("./src/formatTestPath.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const ARROW = ' \u203A ';
const DOWN_ARROW = ' \u21B3 ';
const DOT = ' \u2022 ';
const FAIL_COLOR = _chalk().default.bold.red;
const OBSOLETE_COLOR = _chalk().default.bold.yellow;
const SNAPSHOT_ADDED = _chalk().default.bold.green;
const SNAPSHOT_NOTE = _chalk().default.dim;
const SNAPSHOT_REMOVED = _chalk().default.bold.green;
const SNAPSHOT_SUMMARY = _chalk().default.bold;
const SNAPSHOT_UPDATED = _chalk().default.bold.green;
function getSnapshotSummary(snapshots, globalConfig, updateCommand) {
  const summary = [SNAPSHOT_SUMMARY('Snapshot Summary')];
  if (snapshots.added) {
    summary.push(`${SNAPSHOT_ADDED(`${ARROW + (0, _jestUtil().pluralize)('snapshot', snapshots.added)} written `)}from ${(0, _jestUtil().pluralize)('test suite', snapshots.filesAdded)}.`);
  }
  if (snapshots.unmatched) {
    summary.push(`${FAIL_COLOR(`${ARROW}${(0, _jestUtil().pluralize)('snapshot', snapshots.unmatched)} failed`)} from ${(0, _jestUtil().pluralize)('test suite', snapshots.filesUnmatched)}. ${SNAPSHOT_NOTE(`Inspect your code changes or ${updateCommand} to update them.`)}`);
  }
  if (snapshots.updated) {
    summary.push(`${SNAPSHOT_UPDATED(`${ARROW + (0, _jestUtil().pluralize)('snapshot', snapshots.updated)} updated `)}from ${(0, _jestUtil().pluralize)('test suite', snapshots.filesUpdated)}.`);
  }
  if (snapshots.filesRemoved) {
    if (snapshots.didUpdate) {
      summary.push(`${SNAPSHOT_REMOVED(`${ARROW}${(0, _jestUtil().pluralize)('snapshot file', snapshots.filesRemoved)} removed `)}from ${(0, _jestUtil().pluralize)('test suite', snapshots.filesRemoved)}.`);
    } else {
      summary.push(`${OBSOLETE_COLOR(`${ARROW}${(0, _jestUtil().pluralize)('snapshot file', snapshots.filesRemoved)} obsolete `)}from ${(0, _jestUtil().pluralize)('test suite', snapshots.filesRemoved)}. ${SNAPSHOT_NOTE(`To remove ${snapshots.filesRemoved === 1 ? 'it' : 'them all'}, ${updateCommand}.`)}`);
    }
  }
  if (snapshots.filesRemovedList && snapshots.filesRemovedList.length > 0) {
    const [head, ...tail] = snapshots.filesRemovedList;
    summary.push(`  ${DOWN_ARROW} ${DOT}${(0, _formatTestPath.default)(globalConfig, head)}`);
    for (const key of tail) {
      summary.push(`      ${DOT}${(0, _formatTestPath.default)(globalConfig, key)}`);
    }
  }
  if (snapshots.unchecked) {
    if (snapshots.didUpdate) {
      summary.push(`${SNAPSHOT_REMOVED(`${ARROW}${(0, _jestUtil().pluralize)('snapshot', snapshots.unchecked)} removed `)}from ${(0, _jestUtil().pluralize)('test suite', snapshots.uncheckedKeysByFile.length)}.`);
    } else {
      summary.push(`${OBSOLETE_COLOR(`${ARROW}${(0, _jestUtil().pluralize)('snapshot', snapshots.unchecked)} obsolete `)}from ${(0, _jestUtil().pluralize)('test suite', snapshots.uncheckedKeysByFile.length)}. ${SNAPSHOT_NOTE(`To remove ${snapshots.unchecked === 1 ? 'it' : 'them all'}, ${updateCommand}.`)}`);
    }
    for (const uncheckedFile of snapshots.uncheckedKeysByFile) {
      summary.push(`  ${DOWN_ARROW}${(0, _formatTestPath.default)(globalConfig, uncheckedFile.filePath)}`);
      for (const key of uncheckedFile.keys) {
        summary.push(`      ${DOT}${key}`);
      }
    }
  }
  return summary;
}

/***/ },

/***/ "./src/getSummary.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.PROGRESS_BAR_WIDTH = void 0;
exports["default"] = getSummary;
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

const PROGRESS_BAR_WIDTH = exports.PROGRESS_BAR_WIDTH = 40;
function getValuesCurrentTestCases(currentTestCases = []) {
  let numFailingTests = 0;
  let numPassingTests = 0;
  let numPendingTests = 0;
  let numTodoTests = 0;
  let numTotalTests = 0;
  for (const testCase of currentTestCases) {
    switch (testCase.testCaseResult.status) {
      case 'failed':
        {
          numFailingTests++;
          break;
        }
      case 'passed':
        {
          numPassingTests++;
          break;
        }
      case 'skipped':
        {
          numPendingTests++;
          break;
        }
      case 'todo':
        {
          numTodoTests++;
          break;
        }
    }
    numTotalTests++;
  }
  return {
    numFailingTests,
    numPassingTests,
    numPendingTests,
    numTodoTests,
    numTotalTests
  };
}
function renderTime(runTime, estimatedTime, width) {
  // If we are more than one second over the estimated time, highlight it.
  const renderedTime = estimatedTime && runTime >= estimatedTime + 1 ? _chalk().default.bold.yellow((0, _jestUtil().formatTime)(runTime, 0)) : (0, _jestUtil().formatTime)(runTime, 0);
  let time = `${_chalk().default.bold('Time:')}        ${renderedTime}`;
  if (runTime < estimatedTime) {
    time += `, estimated ${(0, _jestUtil().formatTime)(estimatedTime, 0)}`;
  }

  // Only show a progress bar if the test run is actually going to take
  // some time.
  if (estimatedTime > 2 && runTime < estimatedTime && width) {
    const availableWidth = Math.min(PROGRESS_BAR_WIDTH, width);
    const length = Math.min(Math.floor(runTime / estimatedTime * availableWidth), availableWidth);
    if (availableWidth >= 2) {
      time += `\n${_chalk().default.green('█').repeat(length)}${_chalk().default.white('█').repeat(availableWidth - length)}`;
    }
  }
  return time;
}
function getSummary(aggregatedResults, options) {
  let runTime = (Date.now() - aggregatedResults.startTime) / 1000;
  if (options && options.roundTime) {
    runTime = Math.floor(runTime);
  }
  const valuesForCurrentTestCases = getValuesCurrentTestCases(options?.currentTestCases);
  const estimatedTime = options && options.estimatedTime || 0;
  const snapshotResults = aggregatedResults.snapshot;
  const snapshotsAdded = snapshotResults.added;
  const snapshotsFailed = snapshotResults.unmatched;
  const snapshotsOutdated = snapshotResults.unchecked;
  const snapshotsFilesRemoved = snapshotResults.filesRemoved;
  const snapshotsDidUpdate = snapshotResults.didUpdate;
  const snapshotsPassed = snapshotResults.matched;
  const snapshotsTotal = snapshotResults.total;
  const snapshotsUpdated = snapshotResults.updated;
  const suitesFailed = aggregatedResults.numFailedTestSuites;
  const suitesPassed = aggregatedResults.numPassedTestSuites;
  const suitesPending = aggregatedResults.numPendingTestSuites;
  const suitesRun = suitesFailed + suitesPassed;
  const suitesTotal = aggregatedResults.numTotalTestSuites;
  const testsFailed = aggregatedResults.numFailedTests;
  const testsPassed = aggregatedResults.numPassedTests;
  const testsPending = aggregatedResults.numPendingTests;
  const testsTodo = aggregatedResults.numTodoTests;
  const testsTotal = aggregatedResults.numTotalTests;
  const width = options && options.width || 0;
  const optionalLines = [];
  if (options?.showSeed === true) {
    const {
      seed
    } = options;
    if (seed === undefined) {
      throw new Error('Attempted to display seed but seed value is undefined');
    }
    optionalLines.push(`${_chalk().default.bold('Seed:        ') + seed}`);
  }
  const suites = `${_chalk().default.bold('Test Suites: ') + (suitesFailed ? `${_chalk().default.bold.red(`${suitesFailed} failed`)}, ` : '') + (suitesPending ? `${_chalk().default.bold.yellow(`${suitesPending} skipped`)}, ` : '') + (suitesPassed ? `${_chalk().default.bold.green(`${suitesPassed} passed`)}, ` : '') + (suitesRun === suitesTotal ? suitesTotal : `${suitesRun} of ${suitesTotal}`)} total`;
  const updatedTestsFailed = testsFailed + valuesForCurrentTestCases.numFailingTests;
  const updatedTestsPending = testsPending + valuesForCurrentTestCases.numPendingTests;
  const updatedTestsTodo = testsTodo + valuesForCurrentTestCases.numTodoTests;
  const updatedTestsPassed = testsPassed + valuesForCurrentTestCases.numPassingTests;
  const updatedTestsTotal = testsTotal + valuesForCurrentTestCases.numTotalTests;
  const tests = `${_chalk().default.bold('Tests:       ') + (updatedTestsFailed > 0 ? `${_chalk().default.bold.red(`${updatedTestsFailed} failed`)}, ` : '') + (updatedTestsPending > 0 ? `${_chalk().default.bold.yellow(`${updatedTestsPending} skipped`)}, ` : '') + (updatedTestsTodo > 0 ? `${_chalk().default.bold.magenta(`${updatedTestsTodo} todo`)}, ` : '') + (updatedTestsPassed > 0 ? `${_chalk().default.bold.green(`${updatedTestsPassed} passed`)}, ` : '')}${updatedTestsTotal} total`;
  const snapshots = `${_chalk().default.bold('Snapshots:   ') + (snapshotsFailed ? `${_chalk().default.bold.red(`${snapshotsFailed} failed`)}, ` : '') + (snapshotsOutdated && !snapshotsDidUpdate ? `${_chalk().default.bold.yellow(`${snapshotsOutdated} obsolete`)}, ` : '') + (snapshotsOutdated && snapshotsDidUpdate ? `${_chalk().default.bold.green(`${snapshotsOutdated} removed`)}, ` : '') + (snapshotsFilesRemoved && !snapshotsDidUpdate ? `${_chalk().default.bold.yellow(`${(0, _jestUtil().pluralize)('file', snapshotsFilesRemoved)} obsolete`)}, ` : '') + (snapshotsFilesRemoved && snapshotsDidUpdate ? `${_chalk().default.bold.green(`${(0, _jestUtil().pluralize)('file', snapshotsFilesRemoved)} removed`)}, ` : '') + (snapshotsUpdated ? `${_chalk().default.bold.green(`${snapshotsUpdated} updated`)}, ` : '') + (snapshotsAdded ? `${_chalk().default.bold.green(`${snapshotsAdded} written`)}, ` : '') + (snapshotsPassed ? `${_chalk().default.bold.green(`${snapshotsPassed} passed`)}, ` : '')}${snapshotsTotal} total`;
  const time = renderTime(runTime, estimatedTime, width);
  return [...optionalLines, suites, tests, snapshots, time].join('\n');
}

/***/ },

/***/ "./src/getWatermarks.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getWatermarks;
function istanbulReport() {
  const data = _interopRequireWildcard(require("istanbul-lib-report"));
  istanbulReport = function () {
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

function getWatermarks(config) {
  const defaultWatermarks = istanbulReport().getDefaultWatermarks();
  const {
    coverageThreshold
  } = config;
  if (!coverageThreshold || !coverageThreshold.global) {
    return defaultWatermarks;
  }
  const keys = ['branches', 'functions', 'lines', 'statements'];
  return keys.reduce((watermarks, key) => {
    const value = coverageThreshold.global[key];
    if (value !== undefined) {
      watermarks[key][1] = value;
    }
    return watermarks;
  }, defaultWatermarks);
}

/***/ },

/***/ "./src/printDisplayName.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = printDisplayName;
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

function printDisplayName(config) {
  const {
    displayName
  } = config;
  const white = _chalk().default.reset.inverse.white;
  if (!displayName) {
    return '';
  }
  const {
    name,
    color
  } = displayName;
  const chosenColor = _chalk().default.reset.inverse[color] ?? white;
  return _chalk().default.supportsColor ? chosenColor(` ${name} `) : name;
}

/***/ },

/***/ "./src/relativePath.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = relativePath;
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

function relativePath(config, testPath) {
  // this function can be called with ProjectConfigs or GlobalConfigs. GlobalConfigs
  // do not have config.cwd, only config.rootDir. Try using config.cwd, fallback
  // to config.rootDir. (Also, some unit just use config.rootDir, which is ok)
  testPath = path().relative(config.cwd || config.rootDir, testPath);
  const dirname = path().dirname(testPath);
  const basename = path().basename(testPath);
  return {
    basename,
    dirname
  };
}

/***/ },

/***/ "./src/trimAndFormatPath.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = trimAndFormatPath;
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
function _slash() {
  const data = _interopRequireDefault(require("slash"));
  _slash = function () {
    return data;
  };
  return data;
}
var _relativePath = _interopRequireDefault(__webpack_require__("./src/relativePath.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function trimAndFormatPath(pad, config, testPath, columns) {
  const maxLength = columns - pad;
  const relative = (0, _relativePath.default)(config, testPath);
  const {
    basename
  } = relative;
  let {
    dirname
  } = relative;

  // length is ok
  if ((dirname + path().sep + basename).length <= maxLength) {
    return (0, _slash().default)(_chalk().default.dim(dirname + path().sep) + _chalk().default.bold(basename));
  }

  // we can fit trimmed dirname and full basename
  const basenameLength = basename.length;
  if (basenameLength + 4 < maxLength) {
    const dirnameLength = maxLength - 4 - basenameLength;
    dirname = `...${dirname.slice(dirname.length - dirnameLength)}`;
    return (0, _slash().default)(_chalk().default.dim(dirname + path().sep) + _chalk().default.bold(basename));
  }
  if (basenameLength + 4 === maxLength) {
    return (0, _slash().default)(_chalk().default.dim(`...${path().sep}`) + _chalk().default.bold(basename));
  }

  // can't fit dirname, but can fit trimmed basename
  return (0, _slash().default)(_chalk().default.bold(`...${basename.slice(basename.length - maxLength - 4)}`));
}

/***/ },

/***/ "./src/wrapAnsiString.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = wrapAnsiString;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// word-wrap a string that contains ANSI escape sequences.
// ANSI escape sequences do not add to the string length.
function wrapAnsiString(string, terminalWidth) {
  if (terminalWidth === 0) {
    // if the terminal width is zero, don't bother word-wrapping
    return string;
  }
  const ANSI_REGEXP = /[\u001B\u009B]\[\d{1,2}m/gu;
  const tokens = [];
  let lastIndex = 0;
  let match;
  while (match = ANSI_REGEXP.exec(string)) {
    const ansi = match[0];
    const index = match.index;
    if (index !== lastIndex) {
      tokens.push(['string', string.slice(lastIndex, index)]);
    }
    tokens.push(['ansi', ansi]);
    lastIndex = index + ansi.length;
  }
  if (lastIndex !== string.length - 1) {
    tokens.push(['string', string.slice(lastIndex)]);
  }
  let lastLineLength = 0;
  return tokens.reduce((lines, [kind, token]) => {
    if (kind === 'string') {
      if (lastLineLength + token.length > terminalWidth) {
        while (token.length > 0) {
          const chunk = token.slice(0, terminalWidth - lastLineLength);
          const remaining = token.slice(terminalWidth - lastLineLength);
          lines[lines.length - 1] += chunk;
          lastLineLength += chunk.length;
          token = remaining;
          if (token.length > 0) {
            lines.push('');
            lastLineLength = 0;
          }
        }
      } else {
        lines[lines.length - 1] += token;
        lastLineLength += token.length;
      }
    } else {
      lines[lines.length - 1] += token;
    }
    return lines;
  }, ['']).join('\n');
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
Object.defineProperty(exports, "AgentReporter", ({
  enumerable: true,
  get: function () {
    return _AgentReporter.default;
  }
}));
Object.defineProperty(exports, "BaseReporter", ({
  enumerable: true,
  get: function () {
    return _BaseReporter.default;
  }
}));
Object.defineProperty(exports, "CoverageReporter", ({
  enumerable: true,
  get: function () {
    return _CoverageReporter.default;
  }
}));
Object.defineProperty(exports, "DefaultReporter", ({
  enumerable: true,
  get: function () {
    return _DefaultReporter.default;
  }
}));
Object.defineProperty(exports, "GitHubActionsReporter", ({
  enumerable: true,
  get: function () {
    return _GitHubActionsReporter.default;
  }
}));
Object.defineProperty(exports, "NotifyReporter", ({
  enumerable: true,
  get: function () {
    return _NotifyReporter.default;
  }
}));
Object.defineProperty(exports, "SummaryReporter", ({
  enumerable: true,
  get: function () {
    return _SummaryReporter.default;
  }
}));
Object.defineProperty(exports, "VerboseReporter", ({
  enumerable: true,
  get: function () {
    return _VerboseReporter.default;
  }
}));
exports.utils = void 0;
var _formatTestPath = _interopRequireDefault(__webpack_require__("./src/formatTestPath.ts"));
var _getResultHeader = _interopRequireDefault(__webpack_require__("./src/getResultHeader.ts"));
var _getSnapshotStatus = _interopRequireDefault(__webpack_require__("./src/getSnapshotStatus.ts"));
var _getSnapshotSummary = _interopRequireDefault(__webpack_require__("./src/getSnapshotSummary.ts"));
var _getSummary = _interopRequireDefault(__webpack_require__("./src/getSummary.ts"));
var _printDisplayName = _interopRequireDefault(__webpack_require__("./src/printDisplayName.ts"));
var _relativePath = _interopRequireDefault(__webpack_require__("./src/relativePath.ts"));
var _trimAndFormatPath = _interopRequireDefault(__webpack_require__("./src/trimAndFormatPath.ts"));
var _AgentReporter = _interopRequireDefault(__webpack_require__("./src/AgentReporter.ts"));
var _BaseReporter = _interopRequireDefault(__webpack_require__("./src/BaseReporter.ts"));
var _CoverageReporter = _interopRequireDefault(__webpack_require__("./src/CoverageReporter.ts"));
var _DefaultReporter = _interopRequireDefault(__webpack_require__("./src/DefaultReporter.ts"));
var _GitHubActionsReporter = _interopRequireDefault(__webpack_require__("./src/GitHubActionsReporter.ts"));
var _NotifyReporter = _interopRequireDefault(__webpack_require__("./src/NotifyReporter.ts"));
var _SummaryReporter = _interopRequireDefault(__webpack_require__("./src/SummaryReporter.ts"));
var _VerboseReporter = _interopRequireDefault(__webpack_require__("./src/VerboseReporter.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const utils = exports.utils = {
  formatTestPath: _formatTestPath.default,
  getResultHeader: _getResultHeader.default,
  getSnapshotStatus: _getSnapshotStatus.default,
  getSnapshotSummary: _getSnapshotSummary.default,
  getSummary: _getSummary.default,
  printDisplayName: _printDisplayName.default,
  relativePath: _relativePath.default,
  trimAndFormatPath: _trimAndFormatPath.default
};
})();

module.exports = __webpack_exports__;
/******/ })()
;