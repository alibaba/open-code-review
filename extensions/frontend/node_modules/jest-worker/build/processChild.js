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

/***/ "./src/types.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.WorkerStates = exports.WorkerEvents = exports.PARENT_MESSAGE_SETUP_ERROR = exports.PARENT_MESSAGE_OK = exports.PARENT_MESSAGE_MEM_USAGE = exports.PARENT_MESSAGE_CUSTOM = exports.PARENT_MESSAGE_CLIENT_ERROR = exports.CHILD_MESSAGE_MEM_USAGE = exports.CHILD_MESSAGE_INITIALIZE = exports.CHILD_MESSAGE_END = exports.CHILD_MESSAGE_CALL_SETUP = exports.CHILD_MESSAGE_CALL = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Because of the dynamic nature of a worker communication process, all messages
// coming from any of the other processes cannot be typed. Thus, many types
// include "unknown" as a TS type, which is (unfortunately) correct here.

const CHILD_MESSAGE_INITIALIZE = exports.CHILD_MESSAGE_INITIALIZE = 0;
const CHILD_MESSAGE_CALL = exports.CHILD_MESSAGE_CALL = 1;
const CHILD_MESSAGE_END = exports.CHILD_MESSAGE_END = 2;
const CHILD_MESSAGE_MEM_USAGE = exports.CHILD_MESSAGE_MEM_USAGE = 3;
const CHILD_MESSAGE_CALL_SETUP = exports.CHILD_MESSAGE_CALL_SETUP = 4;
const PARENT_MESSAGE_OK = exports.PARENT_MESSAGE_OK = 0;
const PARENT_MESSAGE_CLIENT_ERROR = exports.PARENT_MESSAGE_CLIENT_ERROR = 1;
const PARENT_MESSAGE_SETUP_ERROR = exports.PARENT_MESSAGE_SETUP_ERROR = 2;
const PARENT_MESSAGE_CUSTOM = exports.PARENT_MESSAGE_CUSTOM = 3;
const PARENT_MESSAGE_MEM_USAGE = exports.PARENT_MESSAGE_MEM_USAGE = 4;

// Option objects.

// Messages passed from the parent to the children.

// Messages passed from the children to the parent.

// Queue types.
let WorkerStates = exports.WorkerStates = /*#__PURE__*/function (WorkerStates) {
  WorkerStates["STARTING"] = "starting";
  WorkerStates["OK"] = "ok";
  WorkerStates["OUT_OF_MEMORY"] = "oom";
  WorkerStates["RESTARTING"] = "restarting";
  WorkerStates["SHUTTING_DOWN"] = "shutting-down";
  WorkerStates["SHUT_DOWN"] = "shut-down";
  return WorkerStates;
}({});
let WorkerEvents = exports.WorkerEvents = /*#__PURE__*/function (WorkerEvents) {
  WorkerEvents["STATE_CHANGE"] = "state-change";
  return WorkerEvents;
}({});

/***/ },

/***/ "./src/workers/safeMessageTransferring.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.packMessage = packMessage;
exports.unpackMessage = unpackMessage;
function _structuredClone() {
  const data = require("@ungap/structured-clone");
  _structuredClone = function () {
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

function packMessage(message) {
  return {
    __STRUCTURED_CLONE_SERIALIZED__: true,
    /**
     * Use the `json: true` option to avoid errors
     * caused by `function` or `symbol` types.
     * It's not ideal to lose `function` and `symbol` types,
     * but reliability is more important.
     */
    data: (0, _structuredClone().serialize)(message, {
      json: true
    })
  };
}
function isTransferringContainer(message) {
  return message != null && typeof message === 'object' && '__STRUCTURED_CLONE_SERIALIZED__' in message && 'data' in message;
}
function unpackMessage(message) {
  if (isTransferringContainer(message)) {
    return (0, _structuredClone().deserialize)(message.data);
  }
  return message;
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


function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
var _types = __webpack_require__("./src/types.ts");
var _safeMessageTransferring = __webpack_require__("./src/workers/safeMessageTransferring.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

let file = null;
let setupArgs = [];
let initialized = false;

/**
 * This file is a small bootstrapper for workers. It sets up the communication
 * between the worker and the parent process, interpreting parent messages and
 * sending results back.
 *
 * The file loaded will be lazily initialized the first time any of the workers
 * is called. This is done for optimal performance: if the farm is initialized,
 * but no call is made to it, child Node processes will be consuming the least
 * possible amount of memory.
 *
 * If an invalid message is detected, the child will exit (by throwing) with a
 * non-zero exit code.
 */
const messageListener = request => {
  switch (request[0]) {
    case _types.CHILD_MESSAGE_INITIALIZE:
      const init = request;
      file = init[2];
      setupArgs = init[3];
      break;
    case _types.CHILD_MESSAGE_CALL:
      const call = request;
      execMethod(call[2], call[3]);
      break;
    case _types.CHILD_MESSAGE_END:
      end();
      break;
    case _types.CHILD_MESSAGE_MEM_USAGE:
      reportMemoryUsage();
      break;
    case _types.CHILD_MESSAGE_CALL_SETUP:
      if (initialized) {
        reportSuccess(void 0);
      } else {
        const main = require(file);
        initialized = true;
        if (main.setup) {
          execFunction(main.setup, main, setupArgs, reportSuccess, reportInitializeError);
        } else {
          reportSuccess(void 0);
        }
      }
      break;
    default:
      throw new TypeError(`Unexpected request from parent process: ${request[0]}`);
  }
};
process.on('message', messageListener);
function reportSuccess(result) {
  if (!process || !process.send) {
    throw new Error('Child can only be used on a forked process');
  }
  try {
    process.send([_types.PARENT_MESSAGE_OK, result]);
  } catch (error) {
    if ((0, _jestUtil().isError)(error) &&
    // if .send is a function, it's a serialization issue
    !error.message.includes('.send is not a function')) {
      // Apply specific serialization only in error cases
      // to avoid affecting performance in regular cases.
      process.send([_types.PARENT_MESSAGE_OK, (0, _safeMessageTransferring.packMessage)(result)]);
    } else {
      throw error;
    }
  }
}
function reportClientError(error) {
  return reportError(error, _types.PARENT_MESSAGE_CLIENT_ERROR);
}
function reportInitializeError(error) {
  return reportError(error, _types.PARENT_MESSAGE_SETUP_ERROR);
}
function reportMemoryUsage() {
  if (!process || !process.send) {
    throw new Error('Child can only be used on a forked process');
  }
  const msg = [_types.PARENT_MESSAGE_MEM_USAGE, process.memoryUsage().heapUsed];
  process.send(msg);
}
function reportError(error, type) {
  if (!process || !process.send) {
    throw new Error('Child can only be used on a forked process');
  }
  if (error == null) {
    error = new Error('"null" or "undefined" thrown');
  }
  process.send([type, error.constructor && error.constructor.name, error.message, error.stack, typeof error === 'object' ? {
    ...error
  } : error]);
}
function end() {
  const main = require(file);
  if (!main.teardown) {
    exitProcess();
    return;
  }
  execFunction(main.teardown, main, [], exitProcess, exitProcess);
}
function exitProcess() {
  // Clean up open handles so the process ideally exits gracefully
  process.removeListener('message', messageListener);
}
function execMethod(method, args) {
  const main = require(file);
  let fn;
  if (method === 'default') {
    fn = main.__esModule ? main.default : main;
  } else {
    fn = main[method];
  }
  function execHelper() {
    execFunction(fn, main, args, reportSuccess, reportClientError);
  }
  if (initialized || !main.setup) {
    execHelper();
    return;
  }
  initialized = true;
  execFunction(main.setup, main, setupArgs, execHelper, reportInitializeError);
}
function execFunction(fn, ctx, args, onResult, onError) {
  let result;
  try {
    result = fn.apply(ctx, args);
  } catch (error) {
    onError(error);
    return;
  }
  if ((0, _jestUtil().isPromise)(result)) {
    result.then(onResult, onError);
  } else {
    onResult(result);
  }
}
module.exports = __webpack_exports__;
/******/ })()
;