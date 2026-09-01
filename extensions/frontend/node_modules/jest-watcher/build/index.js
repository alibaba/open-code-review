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

/***/ "./src/BaseWatchPlugin.ts"
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

class BaseWatchPlugin {
  _stdin;
  _stdout;
  constructor({
    stdin,
    stdout
  }) {
    this._stdin = stdin;
    this._stdout = stdout;
  }

  // eslint-disable-next-line @typescript-eslint/no-empty-function
  apply(_hooks) {}
  getUsageInfo(_globalConfig) {
    return null;
  }

  // eslint-disable-next-line @typescript-eslint/no-empty-function
  onKey(_key) {}
  run(_globalConfig, _updateConfigAndRun) {
    return Promise.resolve();
  }
}
var _default = exports["default"] = BaseWatchPlugin;

/***/ },

/***/ "./src/JestHooks.ts"
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

class JestHooks {
  _listeners;
  _subscriber;
  _emitter;
  constructor() {
    this._listeners = {
      onFileChange: [],
      onTestRunComplete: [],
      shouldRunTestSuite: []
    };
    this._subscriber = {
      onFileChange: fn => {
        this._listeners.onFileChange.push(fn);
      },
      onTestRunComplete: fn => {
        this._listeners.onTestRunComplete.push(fn);
      },
      shouldRunTestSuite: fn => {
        this._listeners.shouldRunTestSuite.push(fn);
      }
    };
    this._emitter = {
      onFileChange: fs => {
        for (const listener of this._listeners.onFileChange) listener(fs);
      },
      onTestRunComplete: results => {
        for (const listener of this._listeners.onTestRunComplete) listener(results);
      },
      shouldRunTestSuite: async testSuiteInfo => {
        const result = await Promise.all(this._listeners.shouldRunTestSuite.map(listener => listener(testSuiteInfo)));
        return result.every(Boolean);
      }
    };
  }
  isUsed(hook) {
    return this._listeners[hook]?.length > 0;
  }
  getSubscriber() {
    return this._subscriber;
  }
  getEmitter() {
    return this._emitter;
  }
}
var _default = exports["default"] = JestHooks;

/***/ },

/***/ "./src/PatternPrompt.ts"
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
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const {
  CLEAR
} = _jestUtil().specialChars;
const usage = entity => `\n${_chalk().default.bold('Pattern Mode Usage')}\n` + ` ${_chalk().default.dim('\u203A Press')} Esc ${_chalk().default.dim('to exit pattern mode.')}\n` + ` ${_chalk().default.dim('\u203A Press')} Enter ` + `${_chalk().default.dim(`to filter by a ${entity} regex pattern.`)}\n` + '\n';
const usageRows = usage('').split('\n').length;
class PatternPrompt {
  _currentUsageRows;
  constructor(_pipe, _prompt, _entityName = '') {
    this._pipe = _pipe;
    this._prompt = _prompt;
    this._entityName = _entityName;
    this._currentUsageRows = usageRows;
  }
  run(onSuccess, onCancel, options) {
    this._pipe.write(_ansiEscapes().default.cursorHide);
    this._pipe.write(CLEAR);
    if (typeof options?.header === 'string' && options.header) {
      this._pipe.write(`${options.header}\n`);
      this._currentUsageRows = usageRows + options.header.split('\n').length;
    } else {
      this._currentUsageRows = usageRows;
    }
    this._pipe.write(usage(this._entityName));
    this._pipe.write(_ansiEscapes().default.cursorShow);
    this._prompt.enter(this._onChange.bind(this), onSuccess, onCancel);
  }
  _onChange(_pattern, _options) {
    this._pipe.write(_ansiEscapes().default.eraseLine);
    this._pipe.write(_ansiEscapes().default.cursorLeft);
  }
}
exports["default"] = PatternPrompt;

/***/ },

/***/ "./src/TestWatcher.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _emittery() {
  const data = _interopRequireDefault(require("emittery"));
  _emittery = function () {
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

class TestWatcher extends _emittery().default {
  state;
  _isWatchMode;
  constructor({
    isWatchMode
  }) {
    super();
    this.state = {
      interrupted: false
    };
    this._isWatchMode = isWatchMode;
  }
  async setState(state) {
    Object.assign(this.state, state);
    await this.emit('change', this.state);
  }
  isInterrupted() {
    return this.state.interrupted;
  }
  isWatchMode() {
    return this._isWatchMode;
  }
}
exports["default"] = TestWatcher;

/***/ },

/***/ "./src/constants.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.KEYS = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const isWindows = process.platform === 'win32';
const KEYS = exports.KEYS = {
  ARROW_DOWN: '\u001B[B',
  ARROW_LEFT: '\u001B[D',
  ARROW_RIGHT: '\u001B[C',
  ARROW_UP: '\u001B[A',
  BACKSPACE: Buffer.from(isWindows ? '08' : '7f', 'hex').toString(),
  CONTROL_C: '\u0003',
  CONTROL_D: '\u0004',
  CONTROL_U: '\u0015',
  ENTER: '\r',
  ESCAPE: '\u001B'
};

/***/ },

/***/ "./src/index.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
var _exportNames = {
  BaseWatchPlugin: true,
  JestHook: true,
  PatternPrompt: true,
  TestWatcher: true,
  Prompt: true
};
Object.defineProperty(exports, "BaseWatchPlugin", ({
  enumerable: true,
  get: function () {
    return _BaseWatchPlugin.default;
  }
}));
Object.defineProperty(exports, "JestHook", ({
  enumerable: true,
  get: function () {
    return _JestHooks.default;
  }
}));
Object.defineProperty(exports, "PatternPrompt", ({
  enumerable: true,
  get: function () {
    return _PatternPrompt.default;
  }
}));
Object.defineProperty(exports, "Prompt", ({
  enumerable: true,
  get: function () {
    return _Prompt.default;
  }
}));
Object.defineProperty(exports, "TestWatcher", ({
  enumerable: true,
  get: function () {
    return _TestWatcher.default;
  }
}));
var _BaseWatchPlugin = _interopRequireDefault(__webpack_require__("./src/BaseWatchPlugin.ts"));
var _JestHooks = _interopRequireDefault(__webpack_require__("./src/JestHooks.ts"));
var _PatternPrompt = _interopRequireDefault(__webpack_require__("./src/PatternPrompt.ts"));
var _TestWatcher = _interopRequireDefault(__webpack_require__("./src/TestWatcher.ts"));
var _constants = __webpack_require__("./src/constants.ts");
Object.keys(_constants).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
  if (key in exports && exports[key] === _constants[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _constants[key];
    }
  });
});
var _Prompt = _interopRequireDefault(__webpack_require__("./src/lib/Prompt.ts"));
var _patternModeHelpers = __webpack_require__("./src/lib/patternModeHelpers.ts");
Object.keys(_patternModeHelpers).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
  if (key in exports && exports[key] === _patternModeHelpers[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _patternModeHelpers[key];
    }
  });
});
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }

/***/ },

/***/ "./src/lib/Prompt.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _constants = __webpack_require__("./src/constants.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class Prompt {
  // Copied from `enter` to satisfy TS
  _entering = true;
  _value = '';
  _onChange;
  _onSuccess;
  _onCancel;
  _offset;
  _promptLength;
  _selection = null;
  constructor() {
    this._offset = -1;
    this._promptLength = 0;

    /* eslint-disable @typescript-eslint/no-empty-function */
    this._onChange = () => {};
    this._onSuccess = () => {};
    this._onCancel = () => {};
    /* eslint-enable */
  }
  _onResize = () => {
    this._onChange();
  };
  enter(onChange, onSuccess, onCancel) {
    this._entering = true;
    this._value = '';
    this._onSuccess = onSuccess;
    this._onCancel = onCancel;
    this._selection = null;
    this._offset = -1;
    this._promptLength = 0;
    this._onChange = () => onChange(this._value, {
      max: 10,
      offset: this._offset
    });
    this._onChange();
    process.stdout.on('resize', this._onResize);
  }
  setPromptLength(length) {
    this._promptLength = length;
  }
  setPromptSelection(selected) {
    this._selection = selected;
  }
  put(key) {
    switch (key) {
      case _constants.KEYS.ENTER:
        this._entering = false;
        this._onSuccess(this._selection ?? this._value);
        this.abort();
        break;
      case _constants.KEYS.ESCAPE:
        this._entering = false;
        this._onCancel(this._value);
        this.abort();
        break;
      case _constants.KEYS.ARROW_DOWN:
        this._offset = Math.min(this._offset + 1, this._promptLength - 1);
        this._onChange();
        break;
      case _constants.KEYS.ARROW_UP:
        this._offset = Math.max(this._offset - 1, -1);
        this._onChange();
        break;
      case _constants.KEYS.ARROW_LEFT:
      case _constants.KEYS.ARROW_RIGHT:
        break;
      case _constants.KEYS.CONTROL_U:
        this._value = '';
        this._offset = -1;
        this._selection = null;
        this._onChange();
        break;
      default:
        this._value = key === _constants.KEYS.BACKSPACE ? this._value.slice(0, -1) : this._value + key;
        this._offset = -1;
        this._selection = null;
        this._onChange();
        break;
    }
  }
  abort() {
    this._entering = false;
    this._value = '';
    process.stdout.removeListener('resize', this._onResize);
  }
  isEntering() {
    return this._entering;
  }
}
exports["default"] = Prompt;

/***/ },

/***/ "./src/lib/patternModeHelpers.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.printPatternCaret = printPatternCaret;
exports.printRestoredPatternCaret = printRestoredPatternCaret;
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
function _stringLength() {
  const data = _interopRequireDefault(require("string-length"));
  _stringLength = function () {
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

function printPatternCaret(pattern, pipe) {
  const inputText = `${_chalk().default.dim(' pattern \u203A')} ${pattern}`;
  pipe.write(_ansiEscapes().default.eraseDown);
  pipe.write(inputText);
  pipe.write(_ansiEscapes().default.cursorSavePosition);
}
function printRestoredPatternCaret(pattern, currentUsageRows, pipe) {
  const inputText = `${_chalk().default.dim(' pattern \u203A')} ${pattern}`;
  pipe.write(_ansiEscapes().default.cursorTo((0, _stringLength().default)(inputText), currentUsageRows - 1));
  pipe.write(_ansiEscapes().default.cursorRestorePosition);
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
/******/ 	
/******/ 	// startup
/******/ 	// Load entry module and return exports
/******/ 	// This entry module is referenced by other modules so it can't be inlined
/******/ 	var __webpack_exports__ = __webpack_require__("./src/index.ts");
/******/ 	module.exports = __webpack_exports__;
/******/ 	
/******/ })()
;