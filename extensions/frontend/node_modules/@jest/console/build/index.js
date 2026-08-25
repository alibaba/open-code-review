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

/***/ "./src/BufferedConsole.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _nodeAssert() {
  const data = require("node:assert");
  _nodeAssert = function () {
    return data;
  };
  return data;
}
function _nodeConsole() {
  const data = require("node:console");
  _nodeConsole = function () {
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

class BufferedConsole extends _nodeConsole().Console {
  _buffer = [];
  _counters = {};
  _timers = {};
  _groupDepth = 0;
  Console = _nodeConsole().Console;
  constructor() {
    super({
      write: message => {
        BufferedConsole.write(this._buffer, 'log', message);
        return true;
      }
    });
  }
  static write(buffer, type, message, stackLevel = 2) {
    const rawStack = new (_jestUtil().ErrorWithStack)(undefined, BufferedConsole.write).stack;
    (0, _jestUtil().invariant)(rawStack != null, 'always have a stack trace');
    const origin = rawStack.split('\n').slice(stackLevel).filter(Boolean).join('\n');
    buffer.push({
      message,
      origin,
      type
    });
    return buffer;
  }
  _log(type, message) {
    BufferedConsole.write(this._buffer, type, '  '.repeat(this._groupDepth) + message, 3);
  }
  assert(value, message) {
    try {
      _nodeAssert().strict.ok(value, message);
    } catch (error) {
      if (!(error instanceof _nodeAssert().AssertionError)) {
        throw error;
      }
      // https://github.com/jestjs/jest/pull/13422#issuecomment-1273396392
      this._log('assert', error.toString().replaceAll(/:\n\n.*\n/gs, ''));
    }
  }
  count(label = 'default') {
    if (!this._counters[label]) {
      this._counters[label] = 0;
    }
    this._log('count', (0, _nodeUtil().format)(`${label}: ${++this._counters[label]}`));
  }
  countReset(label = 'default') {
    this._counters[label] = 0;
  }
  debug(firstArg, ...rest) {
    this._log('debug', (0, _nodeUtil().format)(firstArg, ...rest));
  }
  dir(firstArg, options = {}) {
    const representation = (0, _nodeUtil().inspect)(firstArg, options);
    this._log('dir', (0, _nodeUtil().formatWithOptions)(options, representation));
  }
  dirxml(firstArg, ...rest) {
    this._log('dirxml', (0, _nodeUtil().format)(firstArg, ...rest));
  }
  error(firstArg, ...rest) {
    this._log('error', (0, _nodeUtil().format)(firstArg, ...rest));
  }
  group(title, ...rest) {
    this._groupDepth++;
    if (title != null || rest.length > 0) {
      this._log('group', _chalk().default.bold((0, _nodeUtil().format)(title, ...rest)));
    }
  }
  groupCollapsed(title, ...rest) {
    this._groupDepth++;
    if (title != null || rest.length > 0) {
      this._log('groupCollapsed', _chalk().default.bold((0, _nodeUtil().format)(title, ...rest)));
    }
  }
  groupEnd() {
    if (this._groupDepth > 0) {
      this._groupDepth--;
    }
  }
  info(firstArg, ...rest) {
    this._log('info', (0, _nodeUtil().format)(firstArg, ...rest));
  }
  log(firstArg, ...rest) {
    this._log('log', (0, _nodeUtil().format)(firstArg, ...rest));
  }
  time(label = 'default') {
    if (this._timers[label] != null) {
      return;
    }
    this._timers[label] = new Date();
  }
  timeEnd(label = 'default') {
    const startTime = this._timers[label];
    if (startTime != null) {
      const endTime = new Date();
      const time = endTime.getTime() - startTime.getTime();
      this._log('time', (0, _nodeUtil().format)(`${label}: ${(0, _jestUtil().formatTime)(time)}`));
      delete this._timers[label];
    }
  }
  timeLog(label = 'default', ...data) {
    const startTime = this._timers[label];
    if (startTime != null) {
      const endTime = new Date();
      const time = endTime.getTime() - startTime.getTime();
      this._log('time', (0, _nodeUtil().format)(`${label}: ${(0, _jestUtil().formatTime)(time)}`, ...data));
    }
  }
  warn(firstArg, ...rest) {
    this._log('warn', (0, _nodeUtil().format)(firstArg, ...rest));
  }
  getBuffer() {
    return this._buffer.length > 0 ? this._buffer : undefined;
  }
}
exports["default"] = BufferedConsole;

/***/ },

/***/ "./src/CustomConsole.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _nodeAssert() {
  const data = require("node:assert");
  _nodeAssert = function () {
    return data;
  };
  return data;
}
function _nodeConsole() {
  const data = require("node:console");
  _nodeConsole = function () {
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

class CustomConsole extends _nodeConsole().Console {
  _stdout;
  _stderr;
  _formatBuffer;
  _counters = {};
  _timers = {};
  _groupDepth = 0;
  Console = _nodeConsole().Console;
  constructor(stdout, stderr, formatBuffer = (_type, message) => message) {
    super(stdout, stderr);
    this._stdout = stdout;
    this._stderr = stderr;
    this._formatBuffer = formatBuffer;
  }
  _log(type, message) {
    (0, _jestUtil().clearLine)(this._stdout);
    super.log(this._formatBuffer(type, '  '.repeat(this._groupDepth) + message));
  }
  _logError(type, message) {
    (0, _jestUtil().clearLine)(this._stderr);
    super.error(this._formatBuffer(type, '  '.repeat(this._groupDepth) + message));
  }
  assert(value, message) {
    try {
      _nodeAssert().strict.ok(value, message);
    } catch (error) {
      if (!(error instanceof _nodeAssert().AssertionError)) {
        throw error;
      }
      // https://github.com/jestjs/jest/pull/13422#issuecomment-1273396392
      this._logError('assert', error.toString().replaceAll(/:\n\n.*\n/gs, ''));
    }
  }
  count(label = 'default') {
    if (!this._counters[label]) {
      this._counters[label] = 0;
    }
    this._log('count', (0, _nodeUtil().format)(`${label}: ${++this._counters[label]}`));
  }
  countReset(label = 'default') {
    this._counters[label] = 0;
  }
  debug(firstArg, ...args) {
    this._log('debug', (0, _nodeUtil().format)(firstArg, ...args));
  }
  dir(firstArg, options = {}) {
    const representation = (0, _nodeUtil().inspect)(firstArg, options);
    this._log('dir', (0, _nodeUtil().formatWithOptions)(options, representation));
  }
  dirxml(firstArg, ...args) {
    this._log('dirxml', (0, _nodeUtil().format)(firstArg, ...args));
  }
  error(firstArg, ...args) {
    this._logError('error', (0, _nodeUtil().format)(firstArg, ...args));
  }
  group(title, ...args) {
    this._groupDepth++;
    if (title != null || args.length > 0) {
      this._log('group', _chalk().default.bold((0, _nodeUtil().format)(title, ...args)));
    }
  }
  groupCollapsed(title, ...args) {
    this._groupDepth++;
    if (title != null || args.length > 0) {
      this._log('groupCollapsed', _chalk().default.bold((0, _nodeUtil().format)(title, ...args)));
    }
  }
  groupEnd() {
    if (this._groupDepth > 0) {
      this._groupDepth--;
    }
  }
  info(firstArg, ...args) {
    this._log('info', (0, _nodeUtil().format)(firstArg, ...args));
  }
  log(firstArg, ...args) {
    this._log('log', (0, _nodeUtil().format)(firstArg, ...args));
  }
  time(label = 'default') {
    if (this._timers[label] != null) {
      return;
    }
    this._timers[label] = new Date();
  }
  timeEnd(label = 'default') {
    const startTime = this._timers[label];
    if (startTime != null) {
      const endTime = Date.now();
      const time = endTime - startTime.getTime();
      this._log('time', (0, _nodeUtil().format)(`${label}: ${(0, _jestUtil().formatTime)(time)}`));
      delete this._timers[label];
    }
  }
  timeLog(label = 'default', ...data) {
    const startTime = this._timers[label];
    if (startTime != null) {
      const endTime = new Date();
      const time = endTime.getTime() - startTime.getTime();
      this._log('time', (0, _nodeUtil().format)(`${label}: ${(0, _jestUtil().formatTime)(time)}`, ...data));
    }
  }
  warn(firstArg, ...args) {
    this._logError('warn', (0, _nodeUtil().format)(firstArg, ...args));
  }
  getBuffer() {
    return undefined;
  }
}
exports["default"] = CustomConsole;

/***/ },

/***/ "./src/NullConsole.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _CustomConsole = _interopRequireDefault(__webpack_require__("./src/CustomConsole.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/* eslint-disable @typescript-eslint/no-empty-function */
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class NullConsole extends _CustomConsole.default {
  assert() {}
  debug() {}
  dir() {}
  error() {}
  info() {}
  log() {}
  time() {}
  timeEnd() {}
  timeLog() {}
  trace() {}
  warn() {}
  group() {}
  groupCollapsed() {}
  groupEnd() {}
}
exports["default"] = NullConsole;

/***/ },

/***/ "./src/getConsoleOutput.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = getConsoleOutput;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
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

function getConsoleOutput(buffer, config, globalConfig) {
  const TITLE_INDENT = globalConfig.verbose === true ? ' '.repeat(2) : ' '.repeat(4);
  const CONSOLE_INDENT = TITLE_INDENT + ' '.repeat(2);
  const logEntries = buffer.reduce((output, {
    type,
    message,
    origin
  }) => {
    message = message.split(/\n/).map(line => CONSOLE_INDENT + line).join('\n');
    let typeMessage = `console.${type}`;
    let noStackTrace = true;
    let noCodeFrame = true;
    if (type === 'warn') {
      message = _chalk().default.yellow(message);
      typeMessage = _chalk().default.yellow(typeMessage);
      noStackTrace = globalConfig?.noStackTrace ?? false;
      noCodeFrame = false;
    } else if (type === 'error') {
      message = _chalk().default.red(message);
      typeMessage = _chalk().default.red(typeMessage);
      noStackTrace = globalConfig?.noStackTrace ?? false;
      noCodeFrame = false;
    }
    const options = {
      noCodeFrame,
      noStackTrace
    };
    const formattedStackTrace = (0, _jestMessageUtil().formatStackTrace)(origin, config, options);
    return `${output + TITLE_INDENT + _chalk().default.dim(typeMessage)}\n${message.trimEnd()}\n${_chalk().default.dim(formattedStackTrace.trimEnd())}\n\n`;
  }, '');
  return `${logEntries.trimEnd()}\n`;
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
Object.defineProperty(exports, "BufferedConsole", ({
  enumerable: true,
  get: function () {
    return _BufferedConsole.default;
  }
}));
Object.defineProperty(exports, "CustomConsole", ({
  enumerable: true,
  get: function () {
    return _CustomConsole.default;
  }
}));
Object.defineProperty(exports, "NullConsole", ({
  enumerable: true,
  get: function () {
    return _NullConsole.default;
  }
}));
Object.defineProperty(exports, "getConsoleOutput", ({
  enumerable: true,
  get: function () {
    return _getConsoleOutput.default;
  }
}));
var _BufferedConsole = _interopRequireDefault(__webpack_require__("./src/BufferedConsole.ts"));
var _CustomConsole = _interopRequireDefault(__webpack_require__("./src/CustomConsole.ts"));
var _NullConsole = _interopRequireDefault(__webpack_require__("./src/NullConsole.ts"));
var _getConsoleOutput = _interopRequireDefault(__webpack_require__("./src/getConsoleOutput.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
})();

module.exports = __webpack_exports__;
/******/ })()
;