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

/***/ "./src/InlineSnapshots.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.saveInlineSnapshots = saveInlineSnapshots;
var path = _interopRequireWildcard(require("node:path"));
var fs = _interopRequireWildcard(require("graceful-fs"));
var semver = _interopRequireWildcard(require("semver"));
var _synckit = require("synckit");
var _jestUtil = require("jest-util");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestWriteFile = globalThis[Symbol.for('jest-native-write-file')] || fs.writeFileSync;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const cachedPrettier = new Map();
function saveInlineSnapshots(snapshots, rootDir, prettierPath) {
  let prettier = prettierPath ? cachedPrettier.get(`module|${prettierPath}`) : undefined;
  let workerFn = prettierPath ? cachedPrettier.get(`worker|${prettierPath}`) : undefined;
  if (prettierPath && !prettier) {
    try {
      prettier = require(require.resolve(prettierPath, {
        [Symbol.for('jest-resolve-outside-vm-option')]: true
      }));
      cachedPrettier.set(`module|${prettierPath}`, prettier);
      if (semver.gte(prettier.version, '3.0.0')) {
        workerFn = (0, _synckit.createSyncFn)(require.resolve(/*webpackIgnore: true*/'./worker'));
        cachedPrettier.set(`worker|${prettierPath}`, workerFn);
      }
    } catch (error) {
      if (!(0, _jestUtil.isError)(error)) {
        throw error;
      }
      if (error.code !== 'MODULE_NOT_FOUND') {
        throw error;
      }
    }
  }
  const snapshotsByFile = (0, _utils.groupSnapshotsByFile)(snapshots);
  for (const sourceFilePath of Object.keys(snapshotsByFile)) {
    const {
      sourceFileWithSnapshots,
      snapshotMatcherNames,
      sourceFile
    } = (0, _utils.processInlineSnapshotsWithBabel)(snapshotsByFile[sourceFilePath], sourceFilePath, rootDir);
    let newSourceFile = sourceFileWithSnapshots;
    if (workerFn) {
      newSourceFile = workerFn(prettierPath, sourceFilePath, sourceFileWithSnapshots, snapshotMatcherNames);
    } else if (prettier && semver.gte(prettier.version, '1.5.0')) {
      newSourceFile = runPrettier(prettier, sourceFilePath, sourceFileWithSnapshots, snapshotMatcherNames);
    }
    if (newSourceFile !== sourceFile) {
      jestWriteFile(sourceFilePath, newSourceFile);
    }
  }
}
const runPrettier = (prettier, sourceFilePath, sourceFileWithSnapshots, snapshotMatcherNames) => {
  // Resolve project configuration.
  // For older versions of Prettier, do not load configuration.
  const config = prettier.resolveConfig ? prettier.resolveConfig.sync(sourceFilePath, {
    editorconfig: true
  }) : null;

  // Prioritize parser found in the project config.
  // If not found detect the parser for the test file.
  // For older versions of Prettier, fallback to a simple parser detection.
  // @ts-expect-error - `inferredParser` is `string`
  const inferredParser = typeof config?.parser === 'string' && config.parser || (prettier.getFileInfo ? prettier.getFileInfo.sync(sourceFilePath).inferredParser : simpleDetectParser(sourceFilePath));
  if (!inferredParser) {
    throw new Error(`Could not infer Prettier parser for file ${sourceFilePath}`);
  }

  // Snapshots have now been inserted. Run prettier to make sure that the code is
  // formatted, except snapshot indentation. Snapshots cannot be formatted until
  // after the initial format because we don't know where the call expression
  // will be placed (specifically its indentation), so we have to do two
  // prettier.format calls back-to-back.
  return prettier.format(prettier.format(sourceFileWithSnapshots, {
    ...config,
    filepath: sourceFilePath
  }), {
    ...config,
    filepath: sourceFilePath,
    parser: createFormattingParser(snapshotMatcherNames, inferredParser)
  });
};

// This parser formats snapshots to the correct indentation.
const createFormattingParser = (snapshotMatcherNames, inferredParser) => (text, parsers, options) => {
  // Workaround for https://github.com/prettier/prettier/issues/3150
  options.parser = inferredParser;
  const ast = parsers[inferredParser](text, options);
  (0, _utils.processPrettierAst)(ast, options, snapshotMatcherNames);
  return ast;
};
const simpleDetectParser = filePath => {
  const extname = path.extname(filePath);
  if (/\.tsx?$/.test(extname)) {
    return 'typescript';
  }
  return 'babel';
};

/***/ },

/***/ "./src/SnapshotResolver.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.isSnapshotPath = exports.buildSnapshotResolver = exports.EXTENSION = exports.DOT_EXTENSION = void 0;
var path = _interopRequireWildcard(require("node:path"));
var _chalk = _interopRequireDefault(require("chalk"));
var _transform = require("@jest/transform");
var _jestUtil = require("jest-util");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const EXTENSION = exports.EXTENSION = 'snap';
const DOT_EXTENSION = exports.DOT_EXTENSION = `.${EXTENSION}`;
const isSnapshotPath = path => path.endsWith(DOT_EXTENSION);
exports.isSnapshotPath = isSnapshotPath;
const cache = new Map();
const buildSnapshotResolver = async (config, localRequire = (0, _transform.createTranspilingRequire)(config)) => {
  const key = config.rootDir;
  const resolver = cache.get(key) ?? (await createSnapshotResolver(await localRequire, config.snapshotResolver));
  cache.set(key, resolver);
  return resolver;
};
exports.buildSnapshotResolver = buildSnapshotResolver;
async function createSnapshotResolver(localRequire, snapshotResolverPath) {
  return typeof snapshotResolverPath === 'string' ? createCustomSnapshotResolver(snapshotResolverPath, localRequire) : createDefaultSnapshotResolver();
}
function createDefaultSnapshotResolver() {
  return {
    resolveSnapshotPath: testPath => path.join(path.join(path.dirname(testPath), '__snapshots__'), path.basename(testPath) + DOT_EXTENSION),
    resolveTestPath: snapshotPath => path.resolve(path.dirname(snapshotPath), '..', path.basename(snapshotPath, DOT_EXTENSION)),
    testPathForConsistencyCheck: path.posix.join('consistency_check', '__tests__', 'example.test.js')
  };
}
async function createCustomSnapshotResolver(snapshotResolverPath, localRequire) {
  const custom = (0, _jestUtil.interopRequireDefault)(await localRequire(snapshotResolverPath)).default;
  const keys = [['resolveSnapshotPath', 'function'], ['resolveTestPath', 'function'], ['testPathForConsistencyCheck', 'string']];
  for (const [propName, requiredType] of keys) {
    if (typeof custom[propName] !== requiredType) {
      throw new TypeError(mustImplement(propName, requiredType));
    }
  }
  const customResolver = {
    resolveSnapshotPath: testPath => custom.resolveSnapshotPath(testPath, DOT_EXTENSION),
    resolveTestPath: snapshotPath => custom.resolveTestPath(snapshotPath, DOT_EXTENSION),
    testPathForConsistencyCheck: custom.testPathForConsistencyCheck
  };
  verifyConsistentTransformations(customResolver);
  return customResolver;
}
function mustImplement(propName, requiredType) {
  return `${_chalk.default.bold(`Custom snapshot resolver must implement a \`${propName}\` as a ${requiredType}.`)}\nDocumentation: https://jestjs.io/docs/configuration#snapshotresolver-string`;
}
function verifyConsistentTransformations(custom) {
  const resolvedSnapshotPath = custom.resolveSnapshotPath(custom.testPathForConsistencyCheck);
  const resolvedTestPath = custom.resolveTestPath(resolvedSnapshotPath);
  if (resolvedTestPath !== custom.testPathForConsistencyCheck) {
    throw new Error(_chalk.default.bold(`Custom snapshot resolver functions must transform paths consistently, i.e. expects resolveTestPath(resolveSnapshotPath('${custom.testPathForConsistencyCheck}')) === ${resolvedTestPath}`));
  }
}

/***/ },

/***/ "./src/State.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var fs = _interopRequireWildcard(require("graceful-fs"));
var _snapshotUtils = require("@jest/snapshot-utils");
var _jestMessageUtil = require("jest-message-util");
var _InlineSnapshots = __webpack_require__("./src/InlineSnapshots.ts");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestExistsFile = globalThis[Symbol.for('jest-native-exists-file')] || fs.existsSync;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
class SnapshotState {
  _counters;
  _dirty;
  // @ts-expect-error - seemingly unused?
  _index;
  _updateSnapshot;
  _snapshotData;
  _initialData;
  _snapshotPath;
  _inlineSnapshots;
  _uncheckedKeys;
  _prettierPath;
  _rootDir;
  snapshotFormat;
  added;
  expand;
  matched;
  unmatched;
  updated;
  constructor(snapshotPath, options) {
    this._snapshotPath = snapshotPath;
    const {
      data,
      dirty
    } = (0, _snapshotUtils.getSnapshotData)(this._snapshotPath, options.updateSnapshot);
    this._initialData = data;
    this._snapshotData = data;
    this._dirty = dirty;
    this._prettierPath = options.prettierPath ?? null;
    this._inlineSnapshots = [];
    this._uncheckedKeys = new Set(Object.keys(this._snapshotData));
    this._counters = new Map();
    this._index = 0;
    this.expand = options.expand || false;
    this.added = 0;
    this.matched = 0;
    this.unmatched = 0;
    this._updateSnapshot = options.updateSnapshot;
    this.updated = 0;
    this.snapshotFormat = options.snapshotFormat;
    this._rootDir = options.rootDir;
  }
  markSnapshotsAsCheckedForTest(testName) {
    for (const uncheckedKey of this._uncheckedKeys) {
      if ((0, _snapshotUtils.keyToTestName)(uncheckedKey) === testName) {
        this._uncheckedKeys.delete(uncheckedKey);
      }
    }
  }
  _addSnapshot(key, receivedSerialized, options) {
    this._dirty = true;
    if (options.isInline) {
      // eslint-disable-next-line unicorn/error-message
      const error = options.error || new Error();
      const lines = (0, _jestMessageUtil.getStackTraceLines)((0, _utils.removeLinesBeforeExternalMatcherTrap)(error.stack || ''));
      const frame = (0, _jestMessageUtil.getTopFrame)(lines);
      if (!frame) {
        throw new Error("Jest: Couldn't infer stack frame for inline snapshot.");
      }
      this._inlineSnapshots.push({
        frame,
        snapshot: receivedSerialized
      });
    } else {
      this._snapshotData[key] = receivedSerialized;
    }
  }
  clear() {
    this._snapshotData = this._initialData;
    this._inlineSnapshots = [];
    this._counters = new Map();
    this._index = 0;
    this.added = 0;
    this.matched = 0;
    this.unmatched = 0;
    this.updated = 0;
  }
  save() {
    const hasExternalSnapshots = Object.keys(this._snapshotData).length;
    const hasInlineSnapshots = this._inlineSnapshots.length;
    const isEmpty = !hasExternalSnapshots && !hasInlineSnapshots;
    const status = {
      deleted: false,
      saved: false
    };
    if ((this._dirty || this._uncheckedKeys.size > 0) && !isEmpty) {
      if (hasExternalSnapshots) {
        (0, _snapshotUtils.saveSnapshotFile)(this._snapshotData, this._snapshotPath);
      }
      if (hasInlineSnapshots) {
        (0, _InlineSnapshots.saveInlineSnapshots)(this._inlineSnapshots, this._rootDir, this._prettierPath);
      }
      status.saved = true;
    } else if (!hasExternalSnapshots && jestExistsFile(this._snapshotPath)) {
      if (this._updateSnapshot === 'all') {
        fs.unlinkSync(this._snapshotPath);
      }
      status.deleted = true;
    }
    return status;
  }
  getUncheckedCount() {
    return this._uncheckedKeys.size || 0;
  }
  getUncheckedKeys() {
    return [...this._uncheckedKeys];
  }
  removeUncheckedKeys() {
    if (this._updateSnapshot === 'all' && this._uncheckedKeys.size > 0) {
      this._dirty = true;
      for (const key of this._uncheckedKeys) delete this._snapshotData[key];
      this._uncheckedKeys.clear();
    }
  }
  match({
    testName,
    received,
    key,
    inlineSnapshot,
    isInline,
    error,
    testFailing = false
  }) {
    this._counters.set(testName, (this._counters.get(testName) || 0) + 1);
    const count = Number(this._counters.get(testName));
    if (!key) {
      key = (0, _snapshotUtils.testNameToKey)(testName, count);
    }

    // Do not mark the snapshot as "checked" if the snapshot is inline and
    // there's an external snapshot. This way the external snapshot can be
    // removed with `--updateSnapshot`.
    if (!(isInline && this._snapshotData[key] !== undefined)) {
      this._uncheckedKeys.delete(key);
    }
    const receivedSerialized = (0, _utils.addExtraLineBreaks)((0, _utils.serialize)(received, undefined, this.snapshotFormat));
    const expected = isInline ? inlineSnapshot : this._snapshotData[key];
    const pass = expected === receivedSerialized;
    const hasSnapshot = expected !== undefined;
    const snapshotIsPersisted = isInline || fs.existsSync(this._snapshotPath);
    if (pass && !isInline) {
      // Executing a snapshot file as JavaScript and writing the strings back
      // when other snapshots have changed loses the proper escaping for some
      // characters. Since we check every snapshot in every test, use the newly
      // generated formatted string.
      // Note that this is only relevant when a snapshot is added and the dirty
      // flag is set.
      this._snapshotData[key] = receivedSerialized;
    }

    // In pure matching only runs, return the match result while skipping any updates
    // reports.
    if (testFailing) {
      if (hasSnapshot && !isInline) {
        // Retain current snapshot values.
        this._addSnapshot(key, expected, {
          error,
          isInline
        });
      }
      return {
        actual: (0, _utils.removeExtraLineBreaks)(receivedSerialized),
        count,
        expected: expected === undefined ? undefined : (0, _utils.removeExtraLineBreaks)(expected),
        key,
        pass
      };
    }

    // These are the conditions on when to write snapshots:
    //  * There's no snapshot file in a non-CI environment.
    //  * There is a snapshot file and we decided to update the snapshot.
    //  * There is a snapshot file, but it doesn't have this snapshot.
    // These are the conditions on when not to write snapshots:
    //  * The update flag is set to 'none'.
    //  * There's no snapshot file or a file without this snapshot on a CI environment.
    if (hasSnapshot && this._updateSnapshot === 'all' || (!hasSnapshot || !snapshotIsPersisted) && (this._updateSnapshot === 'new' || this._updateSnapshot === 'all')) {
      if (this._updateSnapshot === 'all') {
        if (pass) {
          this.matched++;
        } else {
          if (hasSnapshot) {
            this.updated++;
          } else {
            this.added++;
          }
          this._addSnapshot(key, receivedSerialized, {
            error,
            isInline
          });
        }
      } else {
        this._addSnapshot(key, receivedSerialized, {
          error,
          isInline
        });
        this.added++;
      }
      return {
        actual: '',
        count,
        expected: '',
        key,
        pass: true
      };
    } else {
      if (pass) {
        this.matched++;
        return {
          actual: '',
          count,
          expected: '',
          key,
          pass: true
        };
      } else {
        this.unmatched++;
        return {
          actual: (0, _utils.removeExtraLineBreaks)(receivedSerialized),
          count,
          expected: expected === undefined ? undefined : (0, _utils.removeExtraLineBreaks)(expected),
          key,
          pass: false
        };
      }
    }
  }
  fail(testName, _received, key) {
    this._counters.set(testName, (this._counters.get(testName) || 0) + 1);
    const count = Number(this._counters.get(testName));
    if (!key) {
      key = (0, _snapshotUtils.testNameToKey)(testName, count);
    }
    this._uncheckedKeys.delete(key);
    this.unmatched++;
    return key;
  }
}
exports["default"] = SnapshotState;

/***/ },

/***/ "./src/colors.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.bForeground3 = exports.bForeground2 = exports.bBackground3 = exports.bBackground2 = exports.aForeground3 = exports.aForeground2 = exports.aBackground3 = exports.aBackground2 = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// https://jonasjacek.github.io/colors/

const aForeground2 = exports.aForeground2 = 90;
const aBackground2 = exports.aBackground2 = 225;
const bForeground2 = exports.bForeground2 = 23;
const bBackground2 = exports.bBackground2 = 195;
const aForeground3 = exports.aForeground3 = [0x80, 0, 0x80];
const aBackground3 = exports.aBackground3 = [0xff, 0xd7, 0xff];
const bForeground3 = exports.bForeground3 = [0, 0x5f, 0x5f];
const bBackground3 = exports.bBackground3 = [0xd7, 0xff, 0xff];

/***/ },

/***/ "./src/dedentLines.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.dedentLines = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const getIndentationLength = line => {
  const result = /^( {2})+/.exec(line);
  return result === null ? 0 : result[0].length;
};
const dedentLine = line => line.slice(getIndentationLength(line));

// Return true if:
// "key": "value has multiple lines\n…
// "key has multiple lines\n…
const hasUnmatchedDoubleQuoteMarks = string => {
  let n = 0;
  let i = string.indexOf('"', 0);
  while (i !== -1) {
    if (i === 0 || string[i - 1] !== '\\') {
      n += 1;
    }
    i = string.indexOf('"', i + 1);
  }
  return n % 2 !== 0;
};
const isFirstLineOfTag = line => /^( {2})*</.test(line);

// The length of the output array is the index of the next input line.

// Push dedented lines of start tag onto output and return true;
// otherwise return false because:
// * props include a multiline string (or text node, if props have markup)
// * start tag does not close
const dedentStartTag = (input, output) => {
  let line = input[output.length];
  output.push(dedentLine(line));
  if (line.includes('>')) {
    return true;
  }
  while (output.length < input.length) {
    line = input[output.length];
    if (hasUnmatchedDoubleQuoteMarks(line)) {
      return false; // because props include a multiline string
    } else if (isFirstLineOfTag(line)) {
      // Recursion only if props have markup.
      if (!dedentMarkup(input, output)) {
        return false;
      }
    } else {
      output.push(dedentLine(line));
      if (line.includes('>')) {
        return true;
      }
    }
  }
  return false;
};

// Push dedented lines of markup onto output and return true;
// otherwise return false because:
// * props include a multiline string
// * text has more than one adjacent line
// * markup does not close
const dedentMarkup = (input, output) => {
  let line = input[output.length];
  if (!dedentStartTag(input, output)) {
    return false;
  }
  if (input[output.length - 1].includes('/>')) {
    return true;
  }
  let isText = false;
  const stack = [getIndentationLength(line)];
  while (stack.length > 0 && output.length < input.length) {
    line = input[output.length];
    if (isFirstLineOfTag(line)) {
      if (line.includes('</')) {
        output.push(dedentLine(line));
        stack.pop();
      } else {
        if (!dedentStartTag(input, output)) {
          return false;
        }
        if (!input[output.length - 1].includes('/>')) {
          stack.push(getIndentationLength(line));
        }
      }
      isText = false;
    } else {
      if (isText) {
        return false; // because text has more than one adjacent line
      }
      const indentationLengthOfTag = stack.at(-1);
      output.push(line.slice(indentationLengthOfTag + 2));
      isText = true;
    }
  }
  return stack.length === 0;
};

// Return lines unindented by heuristic;
// otherwise return null because:
// * props include a multiline string
// * text has more than one adjacent line
// * markup does not close
const dedentLines = input => {
  const output = [];
  while (output.length < input.length) {
    const line = input[output.length];
    if (hasUnmatchedDoubleQuoteMarks(line)) {
      return null;
    } else if (isFirstLineOfTag(line)) {
      if (!dedentMarkup(input, output)) {
        return null;
      }
    } else {
      output.push(dedentLine(line));
    }
  }
  return output;
};
exports.dedentLines = dedentLines;

/***/ },

/***/ "./src/mockSerializer.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.test = exports.serialize = exports["default"] = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const serialize = (val, config, indentation, depth, refs, printer) => {
  // Serialize a non-default name, even if config.printFunctionName is false.
  const name = val.getMockName();
  const nameString = name === 'jest.fn()' ? '' : ` ${name}`;
  let callsString = '';
  if (val.mock.calls.length > 0) {
    const indentationNext = indentation + config.indent;
    callsString = ` {${config.spacingOuter}${indentationNext}"calls": ${printer(val.mock.calls, config, indentationNext, depth, refs)}${config.min ? ', ' : ','}${config.spacingOuter}${indentationNext}"results": ${printer(val.mock.results, config, indentationNext, depth, refs)}${config.min ? '' : ','}${config.spacingOuter}${indentation}}`;
  }
  return `[MockFunction${nameString}]${callsString}`;
};
exports.serialize = serialize;
const test = val => val && !!val._isMockFunction;
exports.test = test;
const plugin = {
  serialize,
  test
};
var _default = exports["default"] = plugin;

/***/ },

/***/ "./src/plugins.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.getSerializers = exports.addSerializer = void 0;
var _prettyFormat = require("pretty-format");
var _mockSerializer = _interopRequireDefault(__webpack_require__("./src/mockSerializer.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const {
  DOMCollection,
  DOMElement,
  Immutable,
  ReactElement,
  ReactTestComponent,
  AsymmetricMatcher
} = _prettyFormat.plugins;
let PLUGINS = [ReactTestComponent, ReactElement, DOMElement, DOMCollection, Immutable, _mockSerializer.default, AsymmetricMatcher];

// Prepend to list so the last added is the first tested.
const addSerializer = plugin => {
  PLUGINS = [plugin, ...PLUGINS];
};
exports.addSerializer = addSerializer;
const getSerializers = () => PLUGINS;
exports.getSerializers = getSerializers;

/***/ },

/***/ "./src/printSnapshot.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.printSnapshotAndReceived = exports.printReceived = exports.printPropertiesAndReceived = exports.printExpected = exports.noColor = exports.matcherHintFromConfig = exports.getSnapshotColorForChalkInstance = exports.getReceivedColorForChalkInstance = exports.bReceivedColor = exports.aSnapshotColor = exports.SNAPSHOT_ARG = exports.PROPERTIES_ARG = exports.HINT_ARG = void 0;
var _chalk = _interopRequireDefault(require("chalk"));
var _expectUtils = require("@jest/expect-utils");
var _getType = require("@jest/get-type");
var _jestDiff = require("jest-diff");
var _jestMatcherUtils = require("jest-matcher-utils");
var _prettyFormat = require("pretty-format");
var _colors = __webpack_require__("./src/colors.ts");
var _dedentLines = __webpack_require__("./src/dedentLines.ts");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const getSnapshotColorForChalkInstance = chalkInstance => {
  const level = chalkInstance.level;
  if (level === 3) {
    return chalkInstance.rgb(_colors.aForeground3[0], _colors.aForeground3[1], _colors.aForeground3[2]).bgRgb(_colors.aBackground3[0], _colors.aBackground3[1], _colors.aBackground3[2]);
  }
  if (level === 2) {
    return chalkInstance.ansi256(_colors.aForeground2).bgAnsi256(_colors.aBackground2);
  }
  return chalkInstance.magenta.bgYellowBright;
};
exports.getSnapshotColorForChalkInstance = getSnapshotColorForChalkInstance;
const getReceivedColorForChalkInstance = chalkInstance => {
  const level = chalkInstance.level;
  if (level === 3) {
    return chalkInstance.rgb(_colors.bForeground3[0], _colors.bForeground3[1], _colors.bForeground3[2]).bgRgb(_colors.bBackground3[0], _colors.bBackground3[1], _colors.bBackground3[2]);
  }
  if (level === 2) {
    return chalkInstance.ansi256(_colors.bForeground2).bgAnsi256(_colors.bBackground2);
  }
  return chalkInstance.cyan.bgWhiteBright; // also known as teal
};
exports.getReceivedColorForChalkInstance = getReceivedColorForChalkInstance;
const aSnapshotColor = exports.aSnapshotColor = getSnapshotColorForChalkInstance(_chalk.default);
const bReceivedColor = exports.bReceivedColor = getReceivedColorForChalkInstance(_chalk.default);
const noColor = string => string;
exports.noColor = noColor;
const HINT_ARG = exports.HINT_ARG = 'hint';
const SNAPSHOT_ARG = exports.SNAPSHOT_ARG = 'snapshot';
const PROPERTIES_ARG = exports.PROPERTIES_ARG = 'properties';
const matcherHintFromConfig = ({
  context: {
    isNot,
    promise
  },
  hint,
  inlineSnapshot,
  matcherName,
  properties
}, isUpdatable) => {
  const options = {
    isNot,
    promise
  };
  if (isUpdatable) {
    options.receivedColor = bReceivedColor;
  }
  let expectedArgument = '';
  if (typeof properties === 'object') {
    expectedArgument = PROPERTIES_ARG;
    if (isUpdatable) {
      options.expectedColor = noColor;
    }
    if (typeof hint === 'string' && hint.length > 0) {
      options.secondArgument = HINT_ARG;
      options.secondArgumentColor = _jestMatcherUtils.BOLD_WEIGHT;
    } else if (typeof inlineSnapshot === 'string') {
      options.secondArgument = SNAPSHOT_ARG;
      if (isUpdatable) {
        options.secondArgumentColor = aSnapshotColor;
      } else {
        options.secondArgumentColor = noColor;
      }
    }
  } else {
    if (typeof hint === 'string' && hint.length > 0) {
      expectedArgument = HINT_ARG;
      options.expectedColor = _jestMatcherUtils.BOLD_WEIGHT;
    } else if (typeof inlineSnapshot === 'string') {
      expectedArgument = SNAPSHOT_ARG;
      if (isUpdatable) {
        options.expectedColor = aSnapshotColor;
      }
    }
  }
  return (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options);
};

// Given array of diffs, return string:
// * include common substrings
// * exclude change substrings which have opposite op
// * include change substrings which have argument op
//   with change color only if there is a common substring
exports.matcherHintFromConfig = matcherHintFromConfig;
const joinDiffs = (diffs, op, hasCommon) => diffs.reduce((reduced, diff) => reduced + (diff[0] === _jestDiff.DIFF_EQUAL ? diff[1] : diff[0] === op ? hasCommon ? (0, _jestMatcherUtils.INVERTED_COLOR)(diff[1]) : diff[1] : ''), '');
const isLineDiffable = received => {
  const receivedType = (0, _getType.getType)(received);
  if ((0, _getType.isPrimitive)(received)) {
    return typeof received === 'string';
  }
  if (receivedType === 'date' || receivedType === 'function' || receivedType === 'regexp') {
    return false;
  }
  if (received instanceof Error) {
    return false;
  }
  if (receivedType === 'object' && typeof received.asymmetricMatch === 'function') {
    return false;
  }
  return true;
};
const printExpected = val => (0, _jestMatcherUtils.EXPECTED_COLOR)((0, _utils.minify)(val));
exports.printExpected = printExpected;
const printReceived = val => (0, _jestMatcherUtils.RECEIVED_COLOR)((0, _utils.minify)(val));
exports.printReceived = printReceived;
const printPropertiesAndReceived = (properties, received, expand // CLI options: true if `--expand` or false if `--no-expand`
) => {
  const aAnnotation = 'Expected properties';
  const bAnnotation = 'Received value';
  if (isLineDiffable(properties) && isLineDiffable(received)) {
    const {
      replacedExpected,
      replacedReceived
    } = (0, _jestMatcherUtils.replaceMatchedToAsymmetricMatcher)(properties, received, [], []);
    return (0, _jestDiff.diffLinesUnified)((0, _utils.serialize)(replacedExpected).split('\n'), (0, _utils.serialize)((0, _expectUtils.getObjectSubset)(replacedReceived, replacedExpected)).split('\n'), {
      aAnnotation,
      aColor: _jestMatcherUtils.EXPECTED_COLOR,
      bAnnotation,
      bColor: _jestMatcherUtils.RECEIVED_COLOR,
      changeLineTrailingSpaceColor: _chalk.default.bgYellow,
      commonLineTrailingSpaceColor: _chalk.default.bgYellow,
      emptyFirstOrLastLinePlaceholder: '↵',
      // U+21B5
      expand,
      includeChangeCounts: true
    });
  }
  const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(aAnnotation, bAnnotation);
  return `${printLabel(aAnnotation) + printExpected(properties)}\n${printLabel(bAnnotation)}${printReceived(received)}`;
};
exports.printPropertiesAndReceived = printPropertiesAndReceived;
const MAX_DIFF_STRING_LENGTH = 20_000;
const printSnapshotAndReceived = (a, b, received, expand, snapshotFormat) => {
  const aAnnotation = 'Snapshot';
  const bAnnotation = 'Received';
  const aColor = aSnapshotColor;
  const bColor = bReceivedColor;
  const options = {
    aAnnotation,
    aColor,
    bAnnotation,
    bColor,
    changeLineTrailingSpaceColor: noColor,
    commonLineTrailingSpaceColor: _chalk.default.bgYellow,
    emptyFirstOrLastLinePlaceholder: '↵',
    // U+21B5
    expand,
    includeChangeCounts: true
  };
  if (typeof received === 'string') {
    if (a.length >= 2 && a.startsWith('"') && a.endsWith('"') && b === (0, _prettyFormat.format)(received)) {
      // If snapshot looks like default serialization of a string
      // and received is string which has default serialization.

      if (!a.includes('\n') && !b.includes('\n')) {
        // If neither string is multiline,
        // display as labels and quoted strings.
        let aQuoted = a;
        let bQuoted = b;
        if (a.length - 2 <= MAX_DIFF_STRING_LENGTH && b.length - 2 <= MAX_DIFF_STRING_LENGTH) {
          const diffs = (0, _jestDiff.diffStringsRaw)(a.slice(1, -1), b.slice(1, -1), true);
          const hasCommon = diffs.some(diff => diff[0] === _jestDiff.DIFF_EQUAL);
          aQuoted = `"${joinDiffs(diffs, _jestDiff.DIFF_DELETE, hasCommon)}"`;
          bQuoted = `"${joinDiffs(diffs, _jestDiff.DIFF_INSERT, hasCommon)}"`;
        }
        const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(aAnnotation, bAnnotation);
        return `${printLabel(aAnnotation) + aColor(aQuoted)}\n${printLabel(bAnnotation)}${bColor(bQuoted)}`;
      }

      // Else either string is multiline, so display as unquoted strings.
      a = (0, _utils.deserializeString)(a); //  hypothetical expected string
      b = received; // not serialized
    }
    // Else expected had custom serialization or was not a string
    // or received has custom serialization.

    return a.length <= MAX_DIFF_STRING_LENGTH && b.length <= MAX_DIFF_STRING_LENGTH ? (0, _jestDiff.diffStringsUnified)(a, b, options) : (0, _jestDiff.diffLinesUnified)(a.split('\n'), b.split('\n'), options);
  }
  if (isLineDiffable(received)) {
    const aLines2 = a.split('\n');
    const bLines2 = b.split('\n');

    // Fall through to fix a regression for custom serializers
    // like jest-snapshot-serializer-raw that ignore the indent option.
    const b0 = (0, _utils.serialize)(received, 0, snapshotFormat);
    if (b0 !== b) {
      const aLines0 = (0, _dedentLines.dedentLines)(aLines2);
      if (aLines0 !== null) {
        // Compare lines without indentation.
        const bLines0 = b0.split('\n');
        return (0, _jestDiff.diffLinesUnified2)(aLines2, bLines2, aLines0, bLines0, options);
      }
    }

    // Fall back because:
    // * props include a multiline string
    // * text has more than one adjacent line
    // * markup does not close
    return (0, _jestDiff.diffLinesUnified)(aLines2, bLines2, options);
  }
  const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(aAnnotation, bAnnotation);
  return `${printLabel(aAnnotation) + aColor(a)}\n${printLabel(bAnnotation)}${bColor(b)}`;
};
exports.printSnapshotAndReceived = printSnapshotAndReceived;

/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.serialize = exports.removeLinesBeforeExternalMatcherTrap = exports.removeExtraLineBreaks = exports.processPrettierAst = exports.processInlineSnapshotsWithBabel = exports.minify = exports.groupSnapshotsByFile = exports.deserializeString = exports.deepMerge = exports.addExtraLineBreaks = void 0;
var fs = _interopRequireWildcard(require("graceful-fs"));
var _snapshotUtils = require("@jest/snapshot-utils");
var _prettyFormat = require("pretty-format");
var _plugins = __webpack_require__("./src/plugins.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestReadFile = globalThis[Symbol.for('jest-native-read-file')] || fs.readFileSync;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
function isObject(item) {
  return item != null && typeof item === 'object' && !Array.isArray(item);
}

// Add extra line breaks at beginning and end of multiline snapshot
// to make the content easier to read.
const addExtraLineBreaks = string => string.includes('\n') ? `\n${string}\n` : string;

// Remove extra line breaks at beginning and end of multiline snapshot.
// Instead of trim, which can remove additional newlines or spaces
// at beginning or end of the content from a custom serializer.
exports.addExtraLineBreaks = addExtraLineBreaks;
const removeExtraLineBreaks = string => string.length > 2 && string.startsWith('\n') && string.endsWith('\n') ? string.slice(1, -1) : string;
exports.removeExtraLineBreaks = removeExtraLineBreaks;
const removeLinesBeforeExternalMatcherTrap = stack => {
  const lines = stack.split('\n');
  for (let i = 0; i < lines.length; i += 1) {
    // It's a function name specified in `packages/expect/src/index.ts`
    // for external custom matchers.
    if (lines[i].includes('__EXTERNAL_MATCHER_TRAP__')) {
      return lines.slice(i + 1).join('\n');
    }
  }
  return stack;
};
exports.removeLinesBeforeExternalMatcherTrap = removeLinesBeforeExternalMatcherTrap;
const escapeRegex = true;
const printFunctionName = false;
const serialize = (val, indent = 2, formatOverrides = {}) => (0, _snapshotUtils.normalizeNewlines)((0, _prettyFormat.format)(val, {
  escapeRegex,
  indent,
  plugins: (0, _plugins.getSerializers)(),
  printFunctionName,
  ...formatOverrides
}));
exports.serialize = serialize;
const minify = val => (0, _prettyFormat.format)(val, {
  escapeRegex,
  min: true,
  plugins: (0, _plugins.getSerializers)(),
  printFunctionName
});

// Remove double quote marks and unescape double quotes and backslashes.
exports.minify = minify;
const deserializeString = stringified => stringified.slice(1, -1).replaceAll(/\\("|\\)/g, '$1');
exports.deserializeString = deserializeString;
const isAnyOrAnything = input => '$$typeof' in input && input.$$typeof === Symbol.for('jest.asymmetricMatcher') && ['Any', 'Anything'].includes(input.constructor.name);
const deepMergeArray = (target, source) => {
  const mergedOutput = [...target];
  for (const [index, sourceElement] of source.entries()) {
    const targetElement = mergedOutput[index];
    if (Array.isArray(target[index]) && Array.isArray(sourceElement)) {
      mergedOutput[index] = deepMergeArray(target[index], sourceElement);
    } else if (isObject(targetElement) && !isAnyOrAnything(sourceElement)) {
      mergedOutput[index] = deepMerge(target[index], sourceElement);
    } else {
      // Source does not exist in target or target is primitive and cannot be deep merged
      mergedOutput[index] = sourceElement;
    }
  }
  return mergedOutput;
};

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
const deepMerge = (target, source) => {
  if (isObject(target) && isObject(source)) {
    const mergedOutput = {
      ...target
    };
    for (const key of Object.keys(source)) {
      if (isObject(source[key]) && !source[key].$$typeof) {
        if (key in target) {
          mergedOutput[key] = deepMerge(target[key], source[key]);
        } else {
          Object.assign(mergedOutput, {
            [key]: source[key]
          });
        }
      } else if (Array.isArray(source[key])) {
        mergedOutput[key] = deepMergeArray(target[key], source[key]);
      } else {
        Object.assign(mergedOutput, {
          [key]: source[key]
        });
      }
    }
    return mergedOutput;
  } else if (Array.isArray(target) && Array.isArray(source)) {
    return deepMergeArray(target, source);
  }
  return target;
};
exports.deepMerge = deepMerge;
const indent = (snapshot, numIndents, indentation) => {
  const lines = snapshot.split('\n');
  // Prevent re-indentation of inline snapshots.
  if (lines.length >= 2 && lines[1].startsWith(indentation.repeat(numIndents + 1))) {
    return snapshot;
  }
  return lines.map((line, index) => {
    if (index === 0) {
      // First line is either a 1-line snapshot or a blank line.
      return line;
    } else if (index === lines.length - 1) {
      // The last line should be placed on the same level as the expect call.
      return indentation.repeat(numIndents) + line;
    } else {
      // Do not indent empty lines.
      if (line === '') {
        return line;
      }

      // Not last line, indent one level deeper than expect call.
      return indentation.repeat(numIndents + 1) + line;
    }
  }).join('\n');
};
const generate = require(require.resolve('@babel/generator', {
  [Symbol.for('jest-resolve-outside-vm-option')]: true
})).default;
const {
  parseSync,
  types
} = require(require.resolve('@babel/core', {
  [Symbol.for('jest-resolve-outside-vm-option')]: true
}));
const {
  isAwaitExpression,
  templateElement,
  templateLiteral,
  traverseFast,
  traverse
} = types;
const processInlineSnapshotsWithBabel = (snapshots, sourceFilePath, rootDir) => {
  const sourceFile = jestReadFile(sourceFilePath, 'utf8');

  // TypeScript projects may not have a babel config; make sure they can be parsed anyway.
  const presets = [require.resolve('babel-preset-current-node-syntax')];
  const plugins = [];
  if (/\.([cm]?ts|tsx)$/.test(sourceFilePath)) {
    plugins.push([require.resolve('@babel/plugin-syntax-typescript'), {
      isTSX: sourceFilePath.endsWith('x')
    },
    // unique name to make sure Babel does not complain about a possible duplicate plugin.
    'TypeScript syntax plugin added by Jest snapshot']);
  }

  // Record the matcher names seen during traversal and pass them down one
  // by one to formatting parser.
  const snapshotMatcherNames = [];
  let ast = null;
  try {
    ast = parseSync(sourceFile, {
      filename: sourceFilePath,
      plugins,
      presets,
      root: rootDir
    });
  } catch (error) {
    // attempt to recover from missing jsx plugin
    if (error.message.includes('@babel/plugin-syntax-jsx')) {
      try {
        const jsxSyntaxPlugin = [require.resolve('@babel/plugin-syntax-jsx'), {},
        // unique name to make sure Babel does not complain about a possible duplicate plugin.
        'JSX syntax plugin added by Jest snapshot'];
        ast = parseSync(sourceFile, {
          filename: sourceFilePath,
          plugins: [...plugins, jsxSyntaxPlugin],
          presets,
          root: rootDir
        });
      } catch {
        throw error;
      }
    } else {
      throw error;
    }
  }
  if (!ast) {
    throw new Error(`jest-snapshot: Failed to parse ${sourceFilePath}`);
  }
  traverseAst(snapshots, ast, snapshotMatcherNames);
  return {
    snapshotMatcherNames,
    sourceFile,
    // substitute in the snapshots in reverse order, so slice calculations aren't thrown off.
    sourceFileWithSnapshots: snapshots.reduceRight((sourceSoFar, nextSnapshot) => {
      const {
        node
      } = nextSnapshot;
      if (!node || typeof node.start !== 'number' || typeof node.end !== 'number') {
        throw new Error('Jest: no snapshot insert location found');
      }

      // A hack to prevent unexpected line breaks in the generated code
      node.loc.end.line = node.loc.start.line;
      return sourceSoFar.slice(0, node.start) + generate(node, {
        retainLines: true
      }).code.trim() + sourceSoFar.slice(node.end);
    }, sourceFile)
  };
};
exports.processInlineSnapshotsWithBabel = processInlineSnapshotsWithBabel;
const processPrettierAst = (ast, options, snapshotMatcherNames, keepNode) => {
  traverse(ast, (node, ancestors) => {
    if (node.type !== 'CallExpression') return;
    const {
      arguments: args,
      callee
    } = node;
    if (callee.type !== 'MemberExpression' || callee.property.type !== 'Identifier' || !snapshotMatcherNames.includes(callee.property.name) || !callee.loc || callee.computed) {
      return;
    }
    let snapshotIndex;
    let snapshot;
    for (const [i, node] of args.entries()) {
      if (node.type === 'TemplateLiteral') {
        snapshotIndex = i;
        snapshot = node.quasis[0].value.raw;
      }
    }
    if (snapshot === undefined) {
      return;
    }
    const parent = ancestors.at(-1).node;
    const startColumn = isAwaitExpression(parent) && parent.loc ? parent.loc.start.column : callee.loc.start.column;
    const useSpaces = !options?.useTabs;
    snapshot = indent(snapshot, Math.ceil(useSpaces ? startColumn / (options?.tabWidth ?? 1) :
    // Each tab is 2 characters.
    startColumn / 2), useSpaces ? ' '.repeat(options?.tabWidth ?? 1) : '\t');
    if (keepNode) {
      args[snapshotIndex].quasis[0].value.raw = snapshot;
    } else {
      const replacementNode = templateLiteral([templateElement({
        raw: snapshot
      })], []);
      args[snapshotIndex] = replacementNode;
    }
  });
};
exports.processPrettierAst = processPrettierAst;
const groupSnapshotsBy = createKey => snapshots => snapshots.reduce((object, inlineSnapshot) => {
  const key = createKey(inlineSnapshot);
  return {
    ...object,
    [key]: [...(object[key] || []), inlineSnapshot]
  };
}, {});
const groupSnapshotsByFrame = groupSnapshotsBy(({
  frame: {
    line,
    column
  }
}) => typeof line === 'number' && typeof column === 'number' ? `${line}:${column - 1}` : '');
const groupSnapshotsByFile = exports.groupSnapshotsByFile = groupSnapshotsBy(({
  frame: {
    file
  }
}) => file);
const traverseAst = (snapshots, ast, snapshotMatcherNames) => {
  const groupedSnapshots = groupSnapshotsByFrame(snapshots);
  const remainingSnapshots = new Set(snapshots.map(({
    snapshot
  }) => snapshot));
  traverseFast(ast, node => {
    if (node.type !== 'CallExpression') return;
    const {
      arguments: args,
      callee
    } = node;
    if (callee.type !== 'MemberExpression' || callee.property.type !== 'Identifier' || callee.property.loc == null) {
      return;
    }
    const {
      line,
      column
    } = callee.property.loc.start;
    const snapshotsForFrame = groupedSnapshots[`${line}:${column}`];
    if (!snapshotsForFrame) {
      return;
    }
    if (snapshotsForFrame.length > 1) {
      throw new Error('Jest: Multiple inline snapshots for the same call are not supported.');
    }
    const inlineSnapshot = snapshotsForFrame[0];
    inlineSnapshot.node = node;
    snapshotMatcherNames.push(callee.property.name);
    const snapshotIndex = args.findIndex(({
      type
    }) => type === 'TemplateLiteral' || type === 'StringLiteral');
    const {
      snapshot
    } = inlineSnapshot;
    remainingSnapshots.delete(snapshot);
    const replacementNode = templateLiteral([templateElement({
      raw: (0, _snapshotUtils.escapeBacktickString)(snapshot)
    })], []);
    if (snapshotIndex === -1) {
      args.push(replacementNode);
    } else {
      args[snapshotIndex] = replacementNode;
    }
  });
  if (remainingSnapshots.size > 0) {
    throw new Error("Jest: Couldn't locate all inline snapshots.");
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
Object.defineProperty(exports, "EXTENSION", ({
  enumerable: true,
  get: function () {
    return _SnapshotResolver.EXTENSION;
  }
}));
Object.defineProperty(exports, "SnapshotState", ({
  enumerable: true,
  get: function () {
    return _State.default;
  }
}));
Object.defineProperty(exports, "addSerializer", ({
  enumerable: true,
  get: function () {
    return _plugins.addSerializer;
  }
}));
Object.defineProperty(exports, "buildSnapshotResolver", ({
  enumerable: true,
  get: function () {
    return _SnapshotResolver.buildSnapshotResolver;
  }
}));
exports.cleanup = void 0;
Object.defineProperty(exports, "getSerializers", ({
  enumerable: true,
  get: function () {
    return _plugins.getSerializers;
  }
}));
Object.defineProperty(exports, "isSnapshotPath", ({
  enumerable: true,
  get: function () {
    return _SnapshotResolver.isSnapshotPath;
  }
}));
exports.toThrowErrorMatchingSnapshot = exports.toThrowErrorMatchingInlineSnapshot = exports.toMatchSnapshot = exports.toMatchInlineSnapshot = void 0;
var fs = _interopRequireWildcard(require("graceful-fs"));
var _snapshotUtils = require("@jest/snapshot-utils");
var _jestMatcherUtils = require("jest-matcher-utils");
var _jestUtil = require("jest-util");
var _SnapshotResolver = __webpack_require__("./src/SnapshotResolver.ts");
var _printSnapshot = __webpack_require__("./src/printSnapshot.ts");
var _utils = __webpack_require__("./src/utils.ts");
var _plugins = __webpack_require__("./src/plugins.ts");
var _State = _interopRequireDefault(__webpack_require__("./src/State.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var src_Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var src_Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestExistsFile = globalThis[src_Symbol.for('jest-native-exists-file')] || fs.existsSync;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const DID_NOT_THROW = 'Received function did not throw'; // same as toThrow
const NOT_SNAPSHOT_MATCHERS = `Snapshot matchers cannot be used with ${(0, _jestMatcherUtils.BOLD_WEIGHT)('not')}`;
const INDENTATION_REGEX = /^([^\S\n]*)\S/m;

// Display name in report when matcher fails same as in snapshot file,
// but with optional hint argument in bold weight.
const printSnapshotName = (concatenatedBlockNames = '', hint = '', count) => {
  const hasNames = concatenatedBlockNames.length > 0;
  const hasHint = hint.length > 0;
  return `Snapshot name: \`${hasNames ? (0, _snapshotUtils.escapeBacktickString)(concatenatedBlockNames) : ''}${hasNames && hasHint ? ': ' : ''}${hasHint ? (0, _jestMatcherUtils.BOLD_WEIGHT)((0, _snapshotUtils.escapeBacktickString)(hint)) : ''} ${count}\``;
};
function stripAddedIndentation(inlineSnapshot) {
  // Find indentation if exists.
  const match = inlineSnapshot.match(INDENTATION_REGEX);
  if (!match || !match[1]) {
    // No indentation.
    return inlineSnapshot;
  }
  const indentation = match[1];
  const lines = inlineSnapshot.split('\n');
  if (lines.length <= 2) {
    // Must be at least 3 lines.
    return inlineSnapshot;
  }
  if (lines[0].trim() !== '' || lines.at(-1).trim() !== '') {
    // If not blank first and last lines, abort.
    return inlineSnapshot;
  }
  for (let i = 1; i < lines.length - 1; i++) {
    if (lines[i] !== '') {
      if (lines[i].indexOf(indentation) !== 0) {
        // All lines except first and last should either be blank or have the same
        // indent as the first line (or more). If this isn't the case we don't
        // want to touch the snapshot at all.
        return inlineSnapshot;
      }
      lines[i] = lines[i].slice(indentation.length);
    }
  }

  // Last line is a special case because it won't have the same indent as others
  // but may still have been given some indent to line up.
  lines[lines.length - 1] = '';

  // Return inline snapshot, now at indent 0.
  inlineSnapshot = lines.join('\n');
  return inlineSnapshot;
}
const fileExists = (filePath, fileSystem) => fileSystem.exists(filePath) || jestExistsFile(filePath);
const cleanup = (fileSystem, update, snapshotResolver, testPathIgnorePatterns) => {
  const pattern = `\\.${_SnapshotResolver.EXTENSION}$`;
  const files = fileSystem.matchFiles(pattern);
  let testIgnorePatternsRegex = null;
  if (testPathIgnorePatterns && testPathIgnorePatterns.length > 0) {
    testIgnorePatternsRegex = new RegExp(testPathIgnorePatterns.join('|'));
  }
  const list = files.filter(snapshotFile => {
    const testPath = snapshotResolver.resolveTestPath(snapshotFile);

    // ignore snapshots of ignored tests
    if (testIgnorePatternsRegex && testIgnorePatternsRegex.test(testPath)) {
      return false;
    }
    if (!fileExists(testPath, fileSystem)) {
      if (update === 'all') {
        fs.unlinkSync(snapshotFile);
      }
      return true;
    }
    return false;
  });
  return {
    filesRemoved: list.length,
    filesRemovedList: list
  };
};
exports.cleanup = cleanup;
const toMatchSnapshot = function (received, propertiesOrHint, hint) {
  const matcherName = 'toMatchSnapshot';
  let properties;
  const length = arguments.length;
  if (length === 2 && typeof propertiesOrHint === 'string') {
    hint = propertiesOrHint;
  } else if (length >= 2) {
    if (typeof propertiesOrHint !== 'object' || propertiesOrHint === null) {
      const options = {
        isNot: this.isNot,
        promise: this.promise
      };
      let printedWithType = (0, _jestMatcherUtils.printWithType)('Expected properties', propertiesOrHint, _printSnapshot.printExpected);
      if (length === 3) {
        options.secondArgument = 'hint';
        options.secondArgumentColor = _jestMatcherUtils.BOLD_WEIGHT;
        if (propertiesOrHint == null) {
          printedWithType += "\n\nTo provide a hint without properties: toMatchSnapshot('hint')";
        }
      }
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, _printSnapshot.PROPERTIES_ARG, options), `Expected ${(0, _jestMatcherUtils.EXPECTED_COLOR)('properties')} must be an object`, printedWithType));
    }

    // Future breaking change: Snapshot hint must be a string
    // if (arguments.length === 3 && typeof hint !== 'string') {}

    properties = propertiesOrHint;
  }
  return _toMatchSnapshot({
    context: this,
    hint,
    isInline: false,
    matcherName,
    properties,
    received
  });
};
exports.toMatchSnapshot = toMatchSnapshot;
const toMatchInlineSnapshot = function (received, propertiesOrSnapshot, inlineSnapshot) {
  const matcherName = 'toMatchInlineSnapshot';
  let properties;
  const length = arguments.length;
  if (length === 2 && typeof propertiesOrSnapshot === 'string') {
    inlineSnapshot = propertiesOrSnapshot;
  } else if (length >= 2) {
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    if (length === 3) {
      options.secondArgument = _printSnapshot.SNAPSHOT_ARG;
      options.secondArgumentColor = _printSnapshot.noColor;
    }
    if (typeof propertiesOrSnapshot !== 'object' || propertiesOrSnapshot === null) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, _printSnapshot.PROPERTIES_ARG, options), `Expected ${(0, _jestMatcherUtils.EXPECTED_COLOR)('properties')} must be an object`, (0, _jestMatcherUtils.printWithType)('Expected properties', propertiesOrSnapshot, _printSnapshot.printExpected)));
    }
    if (length === 3 && typeof inlineSnapshot !== 'string') {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, _printSnapshot.PROPERTIES_ARG, options), 'Inline snapshot must be a string', (0, _jestMatcherUtils.printWithType)('Inline snapshot', inlineSnapshot, _utils.serialize)));
    }
    properties = propertiesOrSnapshot;
  }
  return _toMatchSnapshot({
    context: this,
    inlineSnapshot: inlineSnapshot === undefined ? undefined : stripAddedIndentation(inlineSnapshot),
    isInline: true,
    matcherName,
    properties,
    received
  });
};
exports.toMatchInlineSnapshot = toMatchInlineSnapshot;
const _toMatchSnapshot = config => {
  const {
    context,
    hint,
    inlineSnapshot,
    isInline,
    matcherName,
    properties
  } = config;
  let {
    received
  } = config;

  /** If a test was ran with `test.failing`. Passed by Jest Circus. */
  const {
    testFailing = false
  } = context;
  if (!testFailing && context.dontThrow) {
    // Suppress errors while running tests
    context.dontThrow();
  }
  const {
    currentConcurrentTestName,
    isNot,
    snapshotState
  } = context;
  const currentTestName = currentConcurrentTestName?.() ?? context.currentTestName;
  if (isNot) {
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _printSnapshot.matcherHintFromConfig)(config, false), NOT_SNAPSHOT_MATCHERS));
  }
  if (snapshotState == null) {
    // Because the state is the problem, this is not a matcher error.
    // Call generic stringify from jest-matcher-utils package
    // because uninitialized snapshot state does not need snapshot serializers.
    throw new Error(`${(0, _printSnapshot.matcherHintFromConfig)(config, false)}\n\n` + 'Snapshot state must be initialized' + `\n\n${(0, _jestMatcherUtils.printWithType)('Snapshot state', snapshotState, _jestMatcherUtils.stringify)}`);
  }
  const fullTestName = currentTestName && hint ? `${currentTestName}: ${hint}` : currentTestName || ''; // future BREAKING change: || hint

  if (typeof properties === 'object') {
    if (typeof received !== 'object' || received === null) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _printSnapshot.matcherHintFromConfig)(config, false), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be an object when the matcher has ${(0, _jestMatcherUtils.EXPECTED_COLOR)('properties')}`, (0, _jestMatcherUtils.printWithType)('Received', received, _printSnapshot.printReceived)));
    }
    const propertyPass = context.equals(received, properties, [context.utils.iterableEquality, context.utils.subsetEquality]);
    if (propertyPass) {
      received = (0, _utils.deepMerge)(received, properties);
    } else {
      const key = snapshotState.fail(fullTestName, received);
      const matched = /(\d+)$/.exec(key);
      const count = matched === null ? 1 : Number(matched[1]);
      const message = () => `${(0, _printSnapshot.matcherHintFromConfig)(config, false)}\n\n${printSnapshotName(currentTestName, hint, count)}\n\n${(0, _printSnapshot.printPropertiesAndReceived)(properties, received, snapshotState.expand)}`;
      return {
        message,
        name: matcherName,
        pass: false
      };
    }
  }
  const result = snapshotState.match({
    error: context.error,
    inlineSnapshot,
    isInline,
    received,
    testFailing,
    testName: fullTestName
  });
  const {
    actual,
    count,
    expected,
    pass
  } = result;
  if (pass) {
    return {
      message: () => '',
      pass: true
    };
  }
  const message = expected === undefined ? () => `${(0, _printSnapshot.matcherHintFromConfig)(config, true)}\n\n${printSnapshotName(currentTestName, hint, count)}\n\n` + `New snapshot was ${(0, _jestMatcherUtils.BOLD_WEIGHT)('not written')}. The update flag ` + 'must be explicitly passed to write a new snapshot.\n\n' + 'This is likely because this test is run in a continuous integration ' + '(CI) environment in which snapshots are not written by default.\n\n' + `Received:${actual.includes('\n') ? '\n' : ' '}${(0, _printSnapshot.bReceivedColor)(actual)}` : () => `${(0, _printSnapshot.matcherHintFromConfig)(config, true)}\n\n${printSnapshotName(currentTestName, hint, count)}\n\n${(0, _printSnapshot.printSnapshotAndReceived)(expected, actual, received, snapshotState.expand, snapshotState.snapshotFormat)}`;

  // Passing the actual and expected objects so that a custom reporter
  // could access them, for example in order to display a custom visual diff,
  // or create a different error message
  return {
    actual,
    expected,
    message,
    name: matcherName,
    pass: false
  };
};
const toThrowErrorMatchingSnapshot = function (received, hint, fromPromise) {
  const matcherName = 'toThrowErrorMatchingSnapshot';

  // Future breaking change: Snapshot hint must be a string
  // if (hint !== undefined && typeof hint !== string) {}

  return _toThrowErrorMatchingSnapshot({
    context: this,
    hint,
    isInline: false,
    matcherName,
    received
  }, fromPromise);
};
exports.toThrowErrorMatchingSnapshot = toThrowErrorMatchingSnapshot;
const toThrowErrorMatchingInlineSnapshot = function (received, inlineSnapshot, fromPromise) {
  const matcherName = 'toThrowErrorMatchingInlineSnapshot';
  if (inlineSnapshot !== undefined && typeof inlineSnapshot !== 'string') {
    const options = {
      expectedColor: _printSnapshot.noColor,
      isNot: this.isNot,
      promise: this.promise
    };
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, _printSnapshot.SNAPSHOT_ARG, options), 'Inline snapshot must be a string', (0, _jestMatcherUtils.printWithType)('Inline snapshot', inlineSnapshot, _utils.serialize)));
  }
  return _toThrowErrorMatchingSnapshot({
    context: this,
    inlineSnapshot: inlineSnapshot === undefined ? undefined : stripAddedIndentation(inlineSnapshot),
    isInline: true,
    matcherName,
    received
  }, fromPromise);
};
exports.toThrowErrorMatchingInlineSnapshot = toThrowErrorMatchingInlineSnapshot;
const _toThrowErrorMatchingSnapshot = (config, fromPromise) => {
  const {
    context,
    hint,
    inlineSnapshot,
    isInline,
    matcherName,
    received
  } = config;
  context.dontThrow?.();
  const {
    isNot,
    promise
  } = context;
  if (!fromPromise) {
    if (typeof received !== 'function') {
      const options = {
        isNot,
        promise
      };
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be a function`, (0, _jestMatcherUtils.printWithType)('Received', received, _printSnapshot.printReceived)));
    }
  }
  if (isNot) {
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _printSnapshot.matcherHintFromConfig)(config, false), NOT_SNAPSHOT_MATCHERS));
  }
  let error;
  if (fromPromise) {
    error = received;
  } else {
    try {
      received();
    } catch (receivedError) {
      error = receivedError;
    }
  }
  if (error === undefined) {
    // Because the received value is a function, this is not a matcher error.
    throw new Error(`${(0, _printSnapshot.matcherHintFromConfig)(config, false)}\n\n${DID_NOT_THROW}`);
  }
  let message = error.message;
  while ('cause' in error) {
    error = error.cause;
    if ((0, _jestUtil.isError)(error) || error instanceof Error) {
      message += `\nCause: ${error.message}`;
    } else {
      if (typeof error === 'string') {
        message += `\nCause: ${error}`;
      }
      break;
    }
  }
  return _toMatchSnapshot({
    context,
    hint,
    inlineSnapshot,
    isInline,
    matcherName,
    received: message
  });
};
})();

module.exports = __webpack_exports__;
/******/ })()
;