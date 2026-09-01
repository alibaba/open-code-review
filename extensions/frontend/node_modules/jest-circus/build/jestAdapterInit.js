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

/***/ "./src/eventHandler.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _jestUtil = require("jest-util");
var _globalErrorHandlers = __webpack_require__("./src/globalErrorHandlers.ts");
var _types = __webpack_require__("./src/types.ts");
var _utils = __webpack_require__("./src/utils.ts");
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestNow = globalThis[Symbol.for('jest-native-now')] || globalThis.Date.now;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const eventHandler = (event, state) => {
  switch (event.name) {
    case 'include_test_location_in_result':
      {
        state.includeTestLocationInResult = true;
        break;
      }
    case 'hook_start':
      {
        event.hook.seenDone = false;
        break;
      }
    case 'start_describe_definition':
      {
        const {
          blockName,
          mode
        } = event;
        const {
          currentDescribeBlock,
          currentlyRunningTest
        } = state;
        if (currentlyRunningTest) {
          currentlyRunningTest.errors.push(new Error(`Cannot nest a describe inside a test. Describe block "${blockName}" cannot run because it is nested within "${currentlyRunningTest.name}".`));
          break;
        }
        const describeBlock = (0, _utils.makeDescribe)(blockName, currentDescribeBlock, mode);
        currentDescribeBlock.children.push(describeBlock);
        state.currentDescribeBlock = describeBlock;
        break;
      }
    case 'finish_describe_definition':
      {
        const {
          currentDescribeBlock
        } = state;
        (0, _jestUtil.invariant)(currentDescribeBlock, 'currentDescribeBlock must be there');
        if (!(0, _utils.describeBlockHasTests)(currentDescribeBlock)) {
          for (const hook of currentDescribeBlock.hooks) {
            hook.asyncError.message = `Invalid: ${hook.type}() may not be used in a describe block containing no tests.`;
            state.unhandledErrors.push(hook.asyncError);
          }
        }

        // pass mode of currentDescribeBlock to tests
        // but do not when there is already a single test with "only" mode
        const shouldPassMode = !(currentDescribeBlock.mode === 'only' && currentDescribeBlock.children.some(child => child.type === 'test' && child.mode === 'only'));
        if (shouldPassMode) {
          for (const child of currentDescribeBlock.children) {
            if (child.type === 'test' && !child.mode) {
              child.mode = currentDescribeBlock.mode;
            }
          }
        }
        if (!state.hasFocusedTests && currentDescribeBlock.mode !== 'skip' && currentDescribeBlock.children.some(child => child.type === 'test' && child.mode === 'only')) {
          state.hasFocusedTests = true;
        }
        if (currentDescribeBlock.parent) {
          state.currentDescribeBlock = currentDescribeBlock.parent;
        }
        break;
      }
    case 'add_hook':
      {
        const {
          currentDescribeBlock,
          currentlyRunningTest,
          hasStarted
        } = state;
        const {
          asyncError,
          fn,
          hookType: type,
          timeout
        } = event;
        if (currentlyRunningTest) {
          currentlyRunningTest.errors.push(new Error(`Hooks cannot be defined inside tests. Hook of type "${type}" is nested within "${currentlyRunningTest.name}".`));
          break;
        } else if (hasStarted) {
          state.unhandledErrors.push(new Error('Cannot add a hook after tests have started running. Hooks must be defined synchronously.'));
          break;
        }
        const parent = currentDescribeBlock;
        currentDescribeBlock.hooks.push({
          asyncError,
          fn,
          parent,
          seenDone: false,
          timeout,
          type
        });
        break;
      }
    case 'add_test':
      {
        const {
          currentDescribeBlock,
          currentlyRunningTest,
          hasStarted
        } = state;
        const {
          asyncError,
          fn,
          mode,
          testName: name,
          timeout,
          concurrent,
          failing
        } = event;
        if (currentlyRunningTest) {
          currentlyRunningTest.errors.push(new Error(`Tests cannot be nested. Test "${name}" cannot run because it is nested within "${currentlyRunningTest.name}".`));
          break;
        } else if (hasStarted) {
          state.unhandledErrors.push(new Error('Cannot add a test after tests have started running. Tests must be defined synchronously.'));
          break;
        }
        const test = (0, _utils.makeTest)(fn, mode, concurrent, name, currentDescribeBlock, timeout, asyncError, failing);
        if (currentDescribeBlock.mode !== 'skip' && test.mode === 'only') {
          state.hasFocusedTests = true;
        }
        currentDescribeBlock.children.push(test);
        currentDescribeBlock.tests.push(test);
        break;
      }
    case 'hook_failure':
      {
        const {
          test,
          describeBlock,
          error,
          hook
        } = event;
        const {
          asyncError,
          type
        } = hook;
        if (type === 'beforeAll') {
          (0, _jestUtil.invariant)(describeBlock, 'always present for `*All` hooks');
          (0, _utils.addErrorToEachTestUnderDescribe)(describeBlock, error, asyncError);
        } else if (type === 'afterAll') {
          // Attaching `afterAll` errors to each test makes execution flow
          // too complicated, so we'll consider them to be global.
          state.unhandledErrors.push([error, asyncError]);
        } else {
          (0, _jestUtil.invariant)(test, 'always present for `*Each` hooks');
          test.errors.push([error, asyncError]);
        }
        break;
      }
    case 'test_skip':
      {
        event.test.status = 'skip';
        break;
      }
    case 'test_todo':
      {
        event.test.status = 'todo';
        break;
      }
    case 'test_done':
      {
        event.test.duration = (0, _utils.getTestDuration)(event.test);
        event.test.status = 'done';
        state.currentlyRunningTest = null;
        break;
      }
    case 'test_start':
      {
        state.currentlyRunningTest = event.test;
        event.test.startedAt = jestNow();
        event.test.invocations += 1;
        break;
      }
    case 'test_fn_start':
      {
        event.test.seenDone = false;
        break;
      }
    case 'test_fn_failure':
      {
        const {
          error,
          test: {
            asyncError
          }
        } = event;
        event.test.errors.push([error, asyncError]);
        break;
      }
    case 'test_retry':
      {
        const logErrorsBeforeRetry = globalThis[_types.LOG_ERRORS_BEFORE_RETRY] || false;
        if (logErrorsBeforeRetry) {
          event.test.retryReasons.push(...event.test.errors);
        }
        event.test.errors = [];
        break;
      }
    case 'run_start':
      {
        state.hasStarted = true;
        if (globalThis[_types.TEST_TIMEOUT_SYMBOL]) {
          state.testTimeout = globalThis[_types.TEST_TIMEOUT_SYMBOL];
        }
        break;
      }
    case 'run_finish':
      {
        break;
      }
    case 'setup':
      {
        // Uncaught exception handlers should be defined on the parent process
        // object. If defined on the VM's process object they just no op and let
        // the parent process crash. It might make sense to return a `dispatch`
        // function to the parent process and register handlers there instead, but
        // i'm not sure if this is works. For now i just replicated whatever
        // jasmine was doing -- dabramov
        state.parentProcess = event.parentProcess;
        (0, _jestUtil.invariant)(state.parentProcess);
        state.originalGlobalErrorHandlers = (0, _globalErrorHandlers.injectGlobalErrorHandlers)(state.parentProcess);
        if (event.testNamePattern) {
          state.testNamePattern = new RegExp(event.testNamePattern, 'i');
        }
        break;
      }
    case 'teardown':
      {
        (0, _jestUtil.invariant)(state.originalGlobalErrorHandlers);
        (0, _jestUtil.invariant)(state.parentProcess);
        (0, _globalErrorHandlers.restoreGlobalErrorHandlers)(state.parentProcess, state.originalGlobalErrorHandlers);
        break;
      }
    case 'error':
      {
        // It's very likely for long-running async tests to throw errors. In this
        // case we want to catch them and fail the current test. At the same time
        // there's a possibility that one test sets a long timeout, that will
        // eventually throw after this test finishes but during some other test
        // execution, which will result in one test's error failing another test.
        // In any way, it should be possible to track where the error was thrown
        // from.
        if (state.currentlyRunningTest) {
          if (event.promise) {
            state.currentlyRunningTest.unhandledRejectionErrorByPromise.set(event.promise, event.error);
          } else {
            state.currentlyRunningTest.errors.push(event.error);
          }
        } else {
          if (event.promise) {
            state.unhandledRejectionErrorByPromise.set(event.promise, event.error);
          } else {
            state.unhandledErrors.push(event.error);
          }
        }
        break;
      }
    case 'error_handled':
      {
        if (state.currentlyRunningTest) {
          state.currentlyRunningTest.unhandledRejectionErrorByPromise.delete(event.promise);
        } else {
          state.unhandledRejectionErrorByPromise.delete(event.promise);
        }
        break;
      }
  }
};
var _default = exports["default"] = eventHandler;

/***/ },

/***/ "./src/formatNodeAssertErrors.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _nodeAssert = require("node:assert");
var _chalk = _interopRequireDefault(require("chalk"));
var _jestMatcherUtils = require("jest-matcher-utils");
var _prettyFormat = require("pretty-format");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const assertOperatorsMap = {
  '!=': 'notEqual',
  '!==': 'notStrictEqual',
  '==': 'equal',
  '===': 'strictEqual'
};
const humanReadableOperators = {
  deepEqual: 'to deeply equal',
  deepStrictEqual: 'to deeply and strictly equal',
  equal: 'to be equal',
  notDeepEqual: 'not to deeply equal',
  notDeepStrictEqual: 'not to deeply and strictly equal',
  notEqual: 'to not be equal',
  notStrictEqual: 'not be strictly equal',
  strictEqual: 'to strictly be equal'
};
const formatNodeAssertErrors = (event, state) => {
  if (event.name === 'test_done') {
    event.test.errors = event.test.errors.map(errors => {
      let error;
      if (Array.isArray(errors)) {
        const [originalError, asyncError] = errors;
        if (originalError == null) {
          error = asyncError;
        } else if (originalError.stack) {
          error = originalError;
        } else if (asyncError) {
          error = asyncError;
          error.message = originalError.message || `thrown: ${(0, _prettyFormat.format)(originalError, {
            maxDepth: 3
          })}`;
        } else {
          error = new Error(originalError.message || `thrown: ${(0, _prettyFormat.format)(originalError, {
            maxDepth: 3
          })}`);
        }
      } else {
        error = errors;
      }
      return isAssertionError(error) ? {
        message: assertionErrorMessage(error, {
          expand: state.expand
        })
      } : errors;
    });
  }
};
const getOperatorName = (operator, stack) => {
  if (typeof operator === 'string') {
    return assertOperatorsMap[operator] || operator;
  }
  if (stack.match('.doesNotThrow')) {
    return 'doesNotThrow';
  }
  if (stack.match('.throws')) {
    return 'throws';
  }
  return '';
};
const operatorMessage = operator => {
  const niceOperatorName = getOperatorName(operator, '');
  const humanReadableOperator = humanReadableOperators[niceOperatorName];
  return typeof operator === 'string' ? `${humanReadableOperator || niceOperatorName} to:\n` : '';
};
const assertThrowingMatcherHint = operatorName => operatorName ? _chalk.default.dim('assert') + _chalk.default.dim(`.${operatorName}(`) + _chalk.default.red('function') + _chalk.default.dim(')') : '';
const assertMatcherHint = (operator, operatorName, expected) => {
  let message = '';
  if (operator === '==' && expected === true) {
    message = _chalk.default.dim('assert') + _chalk.default.dim('(') + _chalk.default.red('received') + _chalk.default.dim(')');
  } else if (operatorName) {
    message = _chalk.default.dim('assert') + _chalk.default.dim(`.${operatorName}(`) + _chalk.default.red('received') + _chalk.default.dim(', ') + _chalk.default.green('expected') + _chalk.default.dim(')');
  }
  return message;
};
function assertionErrorMessage(error, options) {
  const {
    expected,
    actual,
    generatedMessage,
    message,
    operator,
    stack
  } = error;
  const diffString = (0, _jestMatcherUtils.diff)(expected, actual, options);
  const hasCustomMessage = !generatedMessage;
  const operatorName = getOperatorName(operator, stack);
  const trimmedStack = stack.replace(message, '').replaceAll(/AssertionError(.*)/g, '');
  if (operatorName === 'doesNotThrow') {
    return (
      // eslint-disable-next-line prefer-template
      buildHintString(assertThrowingMatcherHint(operatorName)) + _chalk.default.reset('Expected the function not to throw an error.\n') + _chalk.default.reset('Instead, it threw:\n') + `  ${(0, _jestMatcherUtils.printReceived)(actual)}` + _chalk.default.reset(hasCustomMessage ? `\n\nMessage:\n  ${message}` : '') + trimmedStack
    );
  }
  if (operatorName === 'throws') {
    if (error.generatedMessage) {
      return buildHintString(assertThrowingMatcherHint(operatorName)) + _chalk.default.reset(error.message) + _chalk.default.reset(hasCustomMessage ? `\n\nMessage:\n  ${message}` : '') + trimmedStack;
    }
    return buildHintString(assertThrowingMatcherHint(operatorName)) + _chalk.default.reset('Expected the function to throw an error.\n') + _chalk.default.reset("But it didn't throw anything.") + _chalk.default.reset(hasCustomMessage ? `\n\nMessage:\n  ${message}` : '') + trimmedStack;
  }
  if (operatorName === 'fail') {
    return buildHintString(assertMatcherHint(operator, operatorName, expected)) + _chalk.default.reset(hasCustomMessage ? `Message:\n  ${message}` : '') + trimmedStack;
  }
  return (
    // eslint-disable-next-line prefer-template
    buildHintString(assertMatcherHint(operator, operatorName, expected)) + _chalk.default.reset(`Expected value ${operatorMessage(operator)}`) + `  ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + _chalk.default.reset('Received:\n') + `  ${(0, _jestMatcherUtils.printReceived)(actual)}` + _chalk.default.reset(hasCustomMessage ? `\n\nMessage:\n  ${message}` : '') + (diffString ? `\n\nDifference:\n\n${diffString}` : '') + trimmedStack
  );
}
function isAssertionError(error) {
  return error && (error instanceof _nodeAssert.AssertionError || error.name === _nodeAssert.AssertionError.name || error.code === 'ERR_ASSERTION');
}
function buildHintString(hint) {
  return hint ? `${hint}\n\n` : '';
}
var _default = exports["default"] = formatNodeAssertErrors;

/***/ },

/***/ "./src/globalErrorHandlers.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.restoreGlobalErrorHandlers = exports.injectGlobalErrorHandlers = void 0;
var _state = __webpack_require__("./src/state.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const uncaughtExceptionListener = error => {
  (0, _state.dispatchSync)({
    error,
    name: 'error'
  });
};
const unhandledRejectionListener = (error, promise) => {
  (0, _state.dispatchSync)({
    error,
    name: 'error',
    promise
  });
};
const rejectionHandledListener = promise => {
  (0, _state.dispatchSync)({
    name: 'error_handled',
    promise
  });
};
const injectGlobalErrorHandlers = parentProcess => {
  const uncaughtException = [...process.listeners('uncaughtException')];
  const unhandledRejection = [...process.listeners('unhandledRejection')];
  const rejectionHandled = [...process.listeners('rejectionHandled')];
  parentProcess.removeAllListeners('uncaughtException');
  parentProcess.removeAllListeners('unhandledRejection');
  parentProcess.removeAllListeners('rejectionHandled');
  parentProcess.on('uncaughtException', uncaughtExceptionListener);
  parentProcess.on('unhandledRejection', unhandledRejectionListener);
  parentProcess.on('rejectionHandled', rejectionHandledListener);
  return {
    rejectionHandled,
    uncaughtException,
    unhandledRejection
  };
};
exports.injectGlobalErrorHandlers = injectGlobalErrorHandlers;
const restoreGlobalErrorHandlers = (parentProcess, originalErrorHandlers) => {
  parentProcess.removeListener('uncaughtException', uncaughtExceptionListener);
  parentProcess.removeListener('unhandledRejection', unhandledRejectionListener);
  parentProcess.removeListener('rejectionHandled', rejectionHandledListener);
  for (const listener of originalErrorHandlers.uncaughtException) {
    parentProcess.on('uncaughtException', listener);
  }
  for (const listener of originalErrorHandlers.unhandledRejection) {
    parentProcess.on('unhandledRejection', listener);
  }
  for (const listener of originalErrorHandlers.rejectionHandled) {
    parentProcess.on('rejectionHandled', listener);
  }
};
exports.restoreGlobalErrorHandlers = restoreGlobalErrorHandlers;

/***/ },

/***/ "./src/index.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
Object.defineProperty(exports, "addEventHandler", ({
  enumerable: true,
  get: function () {
    return _state.addEventHandler;
  }
}));
exports.describe = exports["default"] = exports.beforeEach = exports.beforeAll = exports.afterEach = exports.afterAll = void 0;
Object.defineProperty(exports, "getState", ({
  enumerable: true,
  get: function () {
    return _state.getState;
  }
}));
exports.it = void 0;
Object.defineProperty(exports, "removeEventHandler", ({
  enumerable: true,
  get: function () {
    return _state.removeEventHandler;
  }
}));
Object.defineProperty(exports, "resetState", ({
  enumerable: true,
  get: function () {
    return _state.resetState;
  }
}));
Object.defineProperty(exports, "run", ({
  enumerable: true,
  get: function () {
    return _run.default;
  }
}));
Object.defineProperty(exports, "setState", ({
  enumerable: true,
  get: function () {
    return _state.setState;
  }
}));
exports.test = void 0;
var _jestEach = require("jest-each");
var _jestUtil = require("jest-util");
var _state = __webpack_require__("./src/state.ts");
var _run = _interopRequireDefault(__webpack_require__("./src/run.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const describe = exports.describe = (() => {
  const describe = (blockName, blockFn) => _dispatchDescribe(blockFn, blockName, describe);
  const only = (blockName, blockFn) => _dispatchDescribe(blockFn, blockName, only, 'only');
  const skip = (blockName, blockFn) => _dispatchDescribe(blockFn, blockName, skip, 'skip');
  describe.each = (0, _jestEach.bind)(describe, false);
  only.each = (0, _jestEach.bind)(only, false);
  skip.each = (0, _jestEach.bind)(skip, false);
  describe.only = only;
  describe.skip = skip;
  return describe;
})();
const _dispatchDescribe = (blockFn, blockName, describeFn, mode) => {
  const asyncError = new _jestUtil.ErrorWithStack(undefined, describeFn);
  if (blockFn === undefined) {
    asyncError.message = 'Missing second argument. It must be a callback function.';
    throw asyncError;
  }
  if (typeof blockFn !== 'function') {
    asyncError.message = `Invalid second argument, ${blockFn}. It must be a callback function.`;
    throw asyncError;
  }
  try {
    blockName = (0, _jestUtil.convertDescriptorToString)(blockName);
  } catch (error) {
    asyncError.message = error.message;
    throw asyncError;
  }
  (0, _state.dispatchSync)({
    asyncError,
    blockName,
    mode,
    name: 'start_describe_definition'
  });
  const describeReturn = blockFn();
  if ((0, _jestUtil.isPromise)(describeReturn)) {
    throw new _jestUtil.ErrorWithStack('Returning a Promise from "describe" is not supported. Tests must be defined synchronously.', describeFn);
  } else if (describeReturn !== undefined) {
    throw new _jestUtil.ErrorWithStack('A "describe" callback must not return a value.', describeFn);
  }
  (0, _state.dispatchSync)({
    blockName,
    mode,
    name: 'finish_describe_definition'
  });
};
const _addHook = (fn, hookType, hookFn, timeout) => {
  const asyncError = new _jestUtil.ErrorWithStack(undefined, hookFn);
  if (typeof fn !== 'function') {
    asyncError.message = 'Invalid first argument. It must be a callback function.';
    throw asyncError;
  }
  (0, _state.dispatchSync)({
    asyncError,
    fn,
    hookType,
    name: 'add_hook',
    timeout
  });
};

// Hooks have to pass themselves to the HOF in order for us to trim stack traces.
const beforeEach = (fn, timeout) => _addHook(fn, 'beforeEach', beforeEach, timeout);
exports.beforeEach = beforeEach;
const beforeAll = (fn, timeout) => _addHook(fn, 'beforeAll', beforeAll, timeout);
exports.beforeAll = beforeAll;
const afterEach = (fn, timeout) => _addHook(fn, 'afterEach', afterEach, timeout);
exports.afterEach = afterEach;
const afterAll = (fn, timeout) => _addHook(fn, 'afterAll', afterAll, timeout);
exports.afterAll = afterAll;
const test = exports.test = (() => {
  const test = (testName, fn, timeout) => _addTest(testName, undefined, false, fn, test, timeout);
  const skip = (testName, fn, timeout) => _addTest(testName, 'skip', false, fn, skip, timeout);
  const only = (testName, fn, timeout) => _addTest(testName, 'only', false, fn, test.only, timeout);
  const concurrentTest = (testName, fn, timeout) => _addTest(testName, undefined, true, fn, concurrentTest, timeout);
  const concurrentOnly = (testName, fn, timeout) => _addTest(testName, 'only', true, fn, concurrentOnly, timeout);
  const bindFailing = (concurrent, mode) => {
    const failing = (testName, fn, timeout, eachError) => _addTest(testName, mode, concurrent, fn, failing, timeout, true, eachError);
    failing.each = (0, _jestEach.bind)(failing, false, true);
    return failing;
  };
  test.todo = (testName, ...rest) => {
    if (rest.length > 0 || typeof testName !== 'string') {
      throw new _jestUtil.ErrorWithStack('Todo must be called with only a description.', test.todo);
    }
    // eslint-disable-next-line @typescript-eslint/no-empty-function
    return _addTest(testName, 'todo', false, () => {}, test.todo);
  };
  const _addTest = (testName, mode, concurrent, fn, testFn, timeout, failing, asyncError = new _jestUtil.ErrorWithStack(undefined, testFn)) => {
    try {
      testName = (0, _jestUtil.convertDescriptorToString)(testName);
    } catch (error) {
      asyncError.message = error.message;
      throw asyncError;
    }
    if (fn === undefined) {
      asyncError.message = 'Missing second argument. It must be a callback function. Perhaps you want to use `test.todo` for a test placeholder.';
      throw asyncError;
    }
    if (typeof fn !== 'function') {
      asyncError.message = `Invalid second argument, ${fn}. It must be a callback function.`;
      throw asyncError;
    }
    return (0, _state.dispatchSync)({
      asyncError,
      concurrent,
      failing: failing === undefined ? false : failing,
      fn,
      mode,
      name: 'add_test',
      testName,
      timeout
    });
  };
  test.each = (0, _jestEach.bind)(test);
  only.each = (0, _jestEach.bind)(only);
  skip.each = (0, _jestEach.bind)(skip);
  concurrentTest.each = (0, _jestEach.bind)(concurrentTest, false);
  concurrentOnly.each = (0, _jestEach.bind)(concurrentOnly, false);
  only.failing = bindFailing(false, 'only');
  skip.failing = bindFailing(false, 'skip');
  test.failing = bindFailing(false);
  test.only = only;
  test.skip = skip;
  test.concurrent = concurrentTest;
  concurrentTest.only = concurrentOnly;
  concurrentTest.skip = skip;
  concurrentTest.failing = bindFailing(true);
  concurrentOnly.failing = bindFailing(true, 'only');
  return test;
})();
const it = exports.it = test;
var _default = exports["default"] = {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  it,
  test
};

/***/ },

/***/ "./src/run.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _nodeAsync_hooks = require("node:async_hooks");
var _pLimit = _interopRequireDefault(require("p-limit"));
var _expect = require("@jest/expect");
var _jestUtil = require("jest-util");
var _shuffleArray = _interopRequireWildcard(__webpack_require__("./src/shuffleArray.ts"));
var _state = __webpack_require__("./src/state.ts");
var _types = __webpack_require__("./src/types.ts");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Promise = globalThis[Symbol.for('jest-native-promise')] || globalThis.Promise;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
// Global values can be overwritten by mocks or tests. We'll capture
// the original values in the variables before we require any files.
const {
  setTimeout
} = globalThis;
const testNameStorage = new _nodeAsync_hooks.AsyncLocalStorage();
const run = async () => {
  const {
    rootDescribeBlock,
    seed,
    randomize
  } = (0, _state.getState)();
  _expect.jestExpect.setState({
    currentConcurrentTestName: () => testNameStorage.getStore()
  });
  const rng = randomize ? (0, _shuffleArray.rngBuilder)(seed) : undefined;
  await (0, _state.dispatch)({
    name: 'run_start'
  });
  await _runTestsForDescribeBlock(rootDescribeBlock, rng);
  await (0, _state.dispatch)({
    name: 'run_finish'
  });
  return (0, _utils.makeRunResult)((0, _state.getState)().rootDescribeBlock, (0, _state.getState)().unhandledErrors);
};
function* regroupConcurrentChildren(children) {
  const concurrentTests = children.filter(child => child.type === 'test' && child.concurrent);
  if (concurrentTests.length === 0) {
    yield* children;
    return;
  }
  let collectedConcurrent = false;
  for (const child of children) {
    if (child.type === 'test' && child.concurrent) {
      if (!collectedConcurrent) {
        collectedConcurrent = true;
        yield {
          tests: concurrentTests,
          type: 'test-concurrent'
        };
      }
    } else {
      yield child;
    }
  }
}
const _runTestsForDescribeBlock = async (describeBlock, rng) => {
  await (0, _state.dispatch)({
    describeBlock,
    name: 'run_describe_start'
  });
  const {
    beforeAll,
    afterAll
  } = (0, _utils.getAllHooksForDescribe)(describeBlock);
  const isSkipped = describeBlock.mode === 'skip';
  if (!isSkipped) {
    for (const hook of beforeAll) {
      await _callCircusHook({
        describeBlock,
        hook
      });
    }
  }

  // Tests that fail and are retried we run after other tests
  const retryTimes = Number.parseInt(globalThis[_types.RETRY_TIMES], 10) || 0;
  const hasRetryTimes = retryTimes > 0;
  const waitBeforeRetry = Number.parseInt(globalThis[_types.WAIT_BEFORE_RETRY], 10) || 0;
  const retryImmediately = globalThis[_types.RETRY_IMMEDIATELY] || false;
  const deferredRetryTests = [];
  if (rng) {
    describeBlock.children = (0, _shuffleArray.default)(describeBlock.children, rng);
  }
  // Regroup concurrent tests as a single "sequential" unit
  const children = regroupConcurrentChildren(describeBlock.children);
  const rerunTest = async test => {
    let numRetriesAvailable = retryTimes;
    while (numRetriesAvailable > 0 && test.errors.length > 0) {
      // Clear errors so retries occur
      await (0, _state.dispatch)({
        name: 'test_retry',
        test
      });
      if (waitBeforeRetry > 0) {
        await new Promise(resolve => setTimeout(resolve, waitBeforeRetry));
      }
      await _runTest(test, isSkipped);
      numRetriesAvailable--;
    }
  };
  const handleRetry = async (test, hasErrorsBeforeTestRun, hasRetryTimes) => {
    // no retry if the test passed or had errors before the test ran
    if (test.errors.length === 0 || hasErrorsBeforeTestRun || !hasRetryTimes) {
      return;
    }
    if (!retryImmediately) {
      deferredRetryTests.push(test);
      return;
    }

    // If immediate retry is set, we retry the test immediately after the first run
    await rerunTest(test);
  };
  const runTestWithContext = async child => {
    const hasErrorsBeforeTestRun = child.errors.length > 0;
    return testNameStorage.run((0, _utils.getTestID)(child), async () => {
      await _runTest(child, isSkipped);
      await handleRetry(child, hasErrorsBeforeTestRun, hasRetryTimes);
    });
  };
  for (const child of children) {
    switch (child.type) {
      case 'describeBlock':
        {
          await _runTestsForDescribeBlock(child, rng);
          break;
        }
      case 'test':
        {
          await runTestWithContext(child);
          break;
        }
      case 'test-concurrent':
        {
          await (0, _state.dispatch)({
            describeBlock,
            name: 'concurrent_tests_start',
            tests: child.tests
          });
          const concurrencyLimiter = (0, _pLimit.default)((0, _state.getState)().maxConcurrency);
          const tasks = child.tests.map(concurrentTest => concurrencyLimiter(() => runTestWithContext(concurrentTest)));
          await Promise.all(tasks);
          await (0, _state.dispatch)({
            describeBlock,
            name: 'concurrent_tests_end',
            tests: child.tests
          });
          break;
        }
    }
  }

  // Re-run failed tests n-times if configured
  for (const test of deferredRetryTests) {
    await rerunTest(test);
  }
  if (!isSkipped) {
    for (const hook of afterAll) {
      await _callCircusHook({
        describeBlock,
        hook
      });
    }
  }
  await (0, _state.dispatch)({
    describeBlock,
    name: 'run_describe_finish'
  });
};
const _runTest = async (test, parentSkipped) => {
  await (0, _state.dispatch)({
    name: 'test_start',
    test
  });
  const testContext = Object.create(null);
  const {
    hasFocusedTests,
    testNamePattern
  } = (0, _state.getState)();
  const isSkipped = parentSkipped || test.mode === 'skip' || hasFocusedTests && test.mode === undefined || testNamePattern && !testNamePattern.test((0, _utils.getTestID)(test));
  if (isSkipped) {
    await (0, _state.dispatch)({
      name: 'test_skip',
      test
    });
    return;
  }
  if (test.mode === 'todo') {
    await (0, _state.dispatch)({
      name: 'test_todo',
      test
    });
    return;
  }
  await (0, _state.dispatch)({
    name: 'test_started',
    test
  });
  const {
    afterEach,
    beforeEach
  } = (0, _utils.getEachHooksForTest)(test);
  for (const hook of beforeEach) {
    if (test.errors.length > 0) {
      // If any of the before hooks failed already, we don't run any
      // hooks after that.
      break;
    }
    await _callCircusHook({
      hook,
      test,
      testContext
    });
  }
  await _callCircusTest(test, testContext);
  for (const hook of afterEach) {
    await _callCircusHook({
      hook,
      test,
      testContext
    });
  }

  // `afterAll` hooks should not affect test status (pass or fail), because if
  // we had a global `afterAll` hook it would block all existing tests until
  // this hook is executed. So we dispatch `test_done` right away.
  await (0, _state.dispatch)({
    name: 'test_done',
    test
  });
};
const _callCircusHook = async ({
  hook,
  test,
  describeBlock,
  testContext = {}
}) => {
  await (0, _state.dispatch)({
    hook,
    name: 'hook_start'
  });
  const timeout = hook.timeout || (0, _state.getState)().testTimeout;
  try {
    await (0, _utils.callAsyncCircusFn)(hook, testContext, {
      isHook: true,
      timeout
    });
    await (0, _state.dispatch)({
      describeBlock,
      hook,
      name: 'hook_success',
      test
    });
  } catch (error) {
    await (0, _state.dispatch)({
      describeBlock,
      error,
      hook,
      name: 'hook_failure',
      test
    });
  }
};
const _callCircusTest = async (test, testContext) => {
  await (0, _state.dispatch)({
    name: 'test_fn_start',
    test
  });
  const timeout = test.timeout || (0, _state.getState)().testTimeout;
  (0, _jestUtil.invariant)(test.fn, "Tests with no 'fn' should have 'mode' set to 'skipped'");
  if (test.errors.length > 0) {
    return; // We don't run the test if there's already an error in before hooks.
  }
  try {
    await (0, _utils.callAsyncCircusFn)(test, testContext, {
      isHook: false,
      timeout
    });
    if (test.failing) {
      test.asyncError.message = 'Failing test passed even though it was supposed to fail. Remove `.failing` to remove error.';
      await (0, _state.dispatch)({
        error: test.asyncError,
        name: 'test_fn_failure',
        test
      });
    } else {
      await (0, _state.dispatch)({
        name: 'test_fn_success',
        test
      });
    }
  } catch (error) {
    if (test.failing) {
      await (0, _state.dispatch)({
        name: 'test_fn_success',
        test
      });
    } else {
      await (0, _state.dispatch)({
        error,
        name: 'test_fn_failure',
        test
      });
    }
  }
};
var _default = exports["default"] = run;

/***/ },

/***/ "./src/shuffleArray.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = shuffleArray;
exports.rngBuilder = void 0;
var _pureRand = require("pure-rand");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Generates [from, to] inclusive

const rngBuilder = seed => {
  const gen = (0, _pureRand.xoroshiro128plus)(seed);
  return {
    next: (from, to) => (0, _pureRand.unsafeUniformIntDistribution)(from, to, gen)
  };
};

// Fisher-Yates shuffle
// This is performed in-place
exports.rngBuilder = rngBuilder;
function shuffleArray(array, random) {
  const length = array.length;
  if (length === 0) {
    return [];
  }
  for (let i = 0; i < length; i++) {
    const n = random.next(i, length - 1);
    const value = array[i];
    array[i] = array[n];
    array[n] = value;
  }
  return array;
}

/***/ },

/***/ "./src/state.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.setState = exports.resetState = exports.removeEventHandler = exports.getState = exports.dispatchSync = exports.dispatch = exports.addEventHandler = exports.ROOT_DESCRIBE_BLOCK_NAME = void 0;
var _jestUtil = require("jest-util");
var _eventHandler = _interopRequireDefault(__webpack_require__("./src/eventHandler.ts"));
var _formatNodeAssertErrors = _interopRequireDefault(__webpack_require__("./src/formatNodeAssertErrors.ts"));
var _types = __webpack_require__("./src/types.ts");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const handlers = globalThis[_types.EVENT_HANDLERS] || [_eventHandler.default, _formatNodeAssertErrors.default];
(0, _jestUtil.setGlobal)(globalThis, _types.EVENT_HANDLERS, handlers, 'retain');
const ROOT_DESCRIBE_BLOCK_NAME = exports.ROOT_DESCRIBE_BLOCK_NAME = 'ROOT_DESCRIBE_BLOCK';
const createState = () => {
  const ROOT_DESCRIBE_BLOCK = (0, _utils.makeDescribe)(ROOT_DESCRIBE_BLOCK_NAME);
  return {
    currentDescribeBlock: ROOT_DESCRIBE_BLOCK,
    currentlyRunningTest: null,
    expand: undefined,
    hasFocusedTests: false,
    hasStarted: false,
    includeTestLocationInResult: false,
    maxConcurrency: 5,
    parentProcess: null,
    rootDescribeBlock: ROOT_DESCRIBE_BLOCK,
    seed: 0,
    testNamePattern: null,
    testTimeout: 5000,
    unhandledErrors: [],
    unhandledRejectionErrorByPromise: new Map()
  };
};
const getState = () => globalThis[_types.STATE_SYM];
exports.getState = getState;
const setState = state => {
  (0, _jestUtil.setGlobal)(globalThis, _types.STATE_SYM, state);
  (0, _jestUtil.protectProperties)(state, ['hasFocusedTests', 'hasStarted', 'includeTestLocationInResult', 'maxConcurrency', 'seed', 'testNamePattern', 'testTimeout', 'unhandledErrors', 'unhandledRejectionErrorByPromise']);
  return state;
};
exports.setState = setState;
const resetState = () => {
  setState(createState());
};
exports.resetState = resetState;
resetState();
const dispatch = async event => {
  for (const handler of handlers) {
    await handler(event, getState());
  }
};
exports.dispatch = dispatch;
const dispatchSync = event => {
  for (const handler of handlers) {
    handler(event, getState());
  }
};
exports.dispatchSync = dispatchSync;
const addEventHandler = handler => {
  handlers.push(handler);
};
exports.addEventHandler = addEventHandler;
const removeEventHandler = handler => {
  const index = handlers.lastIndexOf(handler);
  if (index !== -1) {
    handlers.splice(index, 1);
  }
};
exports.removeEventHandler = removeEventHandler;

/***/ },

/***/ "./src/testCaseReportHandler.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _utils = __webpack_require__("./src/utils.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const testCaseReportHandler = (testPath, sendMessageToJest) => event => {
  switch (event.name) {
    case 'test_started':
      {
        const testCaseStartInfo = (0, _utils.createTestCaseStartInfo)(event.test);
        sendMessageToJest('test-case-start', [testPath, testCaseStartInfo]);
        break;
      }
    case 'test_todo':
    case 'test_done':
      {
        const testResult = (0, _utils.makeSingleTestResult)(event.test);
        const testCaseResult = (0, _utils.parseSingleTestResult)(testResult);
        sendMessageToJest('test-case-result', [testPath, testCaseResult]);
        break;
      }
  }
};
var _default = exports["default"] = testCaseReportHandler;

/***/ },

/***/ "./src/types.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.WAIT_BEFORE_RETRY = exports.TEST_TIMEOUT_SYMBOL = exports.STATE_SYM = exports.RETRY_TIMES = exports.RETRY_IMMEDIATELY = exports.LOG_ERRORS_BEFORE_RETRY = exports.EVENT_HANDLERS = void 0;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const STATE_SYM = exports.STATE_SYM = Symbol('JEST_STATE_SYMBOL');
const RETRY_TIMES = exports.RETRY_TIMES = Symbol.for('RETRY_TIMES');
const RETRY_IMMEDIATELY = exports.RETRY_IMMEDIATELY = Symbol.for('RETRY_IMMEDIATELY');
const WAIT_BEFORE_RETRY = exports.WAIT_BEFORE_RETRY = Symbol.for('WAIT_BEFORE_RETRY');
// To pass this value from Runtime object to state we need to use global[sym]
const TEST_TIMEOUT_SYMBOL = exports.TEST_TIMEOUT_SYMBOL = Symbol.for('TEST_TIMEOUT_SYMBOL');
const EVENT_HANDLERS = exports.EVENT_HANDLERS = Symbol.for('EVENT_HANDLERS');
const LOG_ERRORS_BEFORE_RETRY = exports.LOG_ERRORS_BEFORE_RETRY = Symbol.for('LOG_ERRORS_BEFORE_RETRY');

/***/ },

/***/ "./src/unhandledRejectionHandler.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.unhandledRejectionHandler = void 0;
var _jestUtil = require("jest-util");
var _utils = __webpack_require__("./src/utils.ts");
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Promise = globalThis[Symbol.for('jest-native-promise')] || globalThis.Promise;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
// Global values can be overwritten by mocks or tests. We'll capture
// the original values in the variables before we require any files.
const {
  setTimeout
} = globalThis;
const untilNextEventLoopTurn = async () => {
  return new Promise(resolve => {
    setTimeout(resolve, 0);
  });
};
const unhandledRejectionHandler = (runtime, waitForUnhandledRejections) => {
  return async (event, state) => {
    if (event.name === 'hook_start') {
      runtime.enterTestCode();
    } else if (event.name === 'hook_success' || event.name === 'hook_failure') {
      runtime.leaveTestCode();
      if (waitForUnhandledRejections) {
        // We need to give event loop the time to actually execute `rejectionHandled`, `uncaughtException` or `unhandledRejection` events
        await untilNextEventLoopTurn();
      }
      const {
        test,
        describeBlock,
        hook
      } = event;
      const {
        asyncError,
        type
      } = hook;
      if (type === 'beforeAll') {
        (0, _jestUtil.invariant)(describeBlock, 'always present for `*All` hooks');
        for (const error of state.unhandledRejectionErrorByPromise.values()) {
          (0, _utils.addErrorToEachTestUnderDescribe)(describeBlock, error, asyncError);
        }
      } else if (type === 'afterAll') {
        // Attaching `afterAll` errors to each test makes execution flow
        // too complicated, so we'll consider them to be global.
        for (const error of state.unhandledRejectionErrorByPromise.values()) {
          state.unhandledErrors.push([error, asyncError]);
        }
      } else {
        (0, _jestUtil.invariant)(test, 'always present for `*Each` hooks');
        for (const error of test.unhandledRejectionErrorByPromise.values()) {
          test.errors.push([error, asyncError]);
        }
      }
    } else if (event.name === 'test_fn_start') {
      runtime.enterTestCode();
    } else if (event.name === 'test_fn_success' || event.name === 'test_fn_failure') {
      runtime.leaveTestCode();
      if (waitForUnhandledRejections) {
        // We need to give event loop the time to actually execute `rejectionHandled`, `uncaughtException` or `unhandledRejection` events
        await untilNextEventLoopTurn();
      }
      const {
        test
      } = event;
      (0, _jestUtil.invariant)(test, 'always present for `*Each` hooks');
      for (const error of test.unhandledRejectionErrorByPromise.values()) {
        test.errors.push([error, event.test.asyncError]);
      }
    } else if (event.name === 'teardown') {
      if (waitForUnhandledRejections) {
        // We need to give event loop the time to actually execute `rejectionHandled`, `uncaughtException` or `unhandledRejection` events
        await untilNextEventLoopTurn();
      }
      state.unhandledErrors.push(...state.unhandledRejectionErrorByPromise.values());
    }
  };
};
exports.unhandledRejectionHandler = unhandledRejectionHandler;

/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.parseSingleTestResult = exports.makeTest = exports.makeSingleTestResult = exports.makeRunResult = exports.makeDescribe = exports.getTestID = exports.getTestDuration = exports.getEachHooksForTest = exports.getAllHooksForDescribe = exports.describeBlockHasTests = exports.createTestCaseStartInfo = exports.callAsyncCircusFn = exports.addErrorToEachTestUnderDescribe = void 0;
var path = _interopRequireWildcard(require("node:path"));
var _co = _interopRequireDefault(require("co"));
var _dedent = _interopRequireDefault(require("dedent"));
var _isGeneratorFn = _interopRequireDefault(require("is-generator-fn"));
var _slash = _interopRequireDefault(require("slash"));
var _stackUtils = _interopRequireDefault(require("stack-utils"));
var _jestUtil = require("jest-util");
var _prettyFormat = require("pretty-format");
var _state = __webpack_require__("./src/state.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestNow = globalThis[Symbol.for('jest-native-now')] || globalThis.Date.now;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Promise = globalThis[Symbol.for('jest-native-promise')] || globalThis.Promise;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const stackUtils = new _stackUtils.default({
  cwd: 'A path that does not exist'
});
const jestEachBuildDir = (0, _slash.default)(path.dirname(require.resolve('jest-each')));
function takesDoneCallback(fn) {
  return fn.length > 0;
}
function isGeneratorFunction(fn) {
  return (0, _isGeneratorFn.default)(fn);
}
const makeDescribe = (name, parent, mode) => {
  let _mode = mode;
  if (parent && !mode) {
    // If not set explicitly, inherit from the parent describe.
    _mode = parent.mode;
  }
  return {
    type: 'describeBlock',
    // eslint-disable-next-line sort-keys
    children: [],
    hooks: [],
    mode: _mode,
    name: (0, _jestUtil.convertDescriptorToString)(name),
    parent,
    tests: []
  };
};
exports.makeDescribe = makeDescribe;
const makeTest = (fn, mode, concurrent, name, parent, timeout, asyncError, failing) => ({
  type: 'test',
  // eslint-disable-next-line sort-keys
  asyncError,
  concurrent,
  duration: null,
  errors: [],
  failing,
  fn,
  invocations: 0,
  mode,
  name: (0, _jestUtil.convertDescriptorToString)(name),
  numPassingAsserts: 0,
  parent,
  retryReasons: [],
  seenDone: false,
  startedAt: null,
  status: null,
  timeout,
  unhandledRejectionErrorByPromise: new Map()
});

// Traverse the tree of describe blocks and return true if at least one describe
// block has an enabled test.
exports.makeTest = makeTest;
const hasEnabledTest = describeBlock => {
  const {
    hasFocusedTests,
    testNamePattern
  } = (0, _state.getState)();
  return describeBlock.children.some(child => child.type === 'describeBlock' ? hasEnabledTest(child) : !(child.mode === 'skip' || hasFocusedTests && child.mode !== 'only' || testNamePattern && !testNamePattern.test(getTestID(child))));
};
const getAllHooksForDescribe = describe => {
  const result = {
    afterAll: [],
    beforeAll: []
  };
  if (hasEnabledTest(describe)) {
    for (const hook of describe.hooks) {
      switch (hook.type) {
        case 'beforeAll':
          result.beforeAll.push(hook);
          break;
        case 'afterAll':
          result.afterAll.push(hook);
          break;
      }
    }
  }
  return result;
};
exports.getAllHooksForDescribe = getAllHooksForDescribe;
const getEachHooksForTest = test => {
  const result = {
    afterEach: [],
    beforeEach: []
  };
  if (test.concurrent) {
    // *Each hooks are not run for concurrent tests
    return result;
  }
  let block = test.parent;
  do {
    const beforeEachForCurrentBlock = [];
    for (const hook of block.hooks) {
      switch (hook.type) {
        case 'beforeEach':
          beforeEachForCurrentBlock.push(hook);
          break;
        case 'afterEach':
          result.afterEach.push(hook);
          break;
      }
    }
    // 'beforeEach' hooks are executed from top to bottom, the opposite of the
    // way we traversed it.
    result.beforeEach.unshift(...beforeEachForCurrentBlock);
  } while (block = block.parent);
  return result;
};
exports.getEachHooksForTest = getEachHooksForTest;
const describeBlockHasTests = describe => describe.children.some(child => child.type === 'test' || describeBlockHasTests(child));
exports.describeBlockHasTests = describeBlockHasTests;
const _makeTimeoutMessage = (timeout, isHook, takesDoneCallback) => `Exceeded timeout of ${(0, _jestUtil.formatTime)(timeout)} for a ${isHook ? 'hook' : 'test'}${takesDoneCallback ? ' while waiting for `done()` to be called' : ''}.\nAdd a timeout value to this test to increase the timeout, if this is a long-running test. See https://jestjs.io/docs/api#testname-fn-timeout.`;

// Global values can be overwritten by mocks or tests. We'll capture
// the original values in the variables before we require any files.
const {
  setTimeout,
  clearTimeout
} = globalThis;
function checkIsError(error) {
  return !!(error && error.message && error.stack);
}
const callAsyncCircusFn = (testOrHook, testContext, {
  isHook,
  timeout
}) => {
  let timeoutID;
  let completed = false;
  const {
    fn,
    asyncError
  } = testOrHook;
  const doneCallback = takesDoneCallback(fn);
  return new Promise((resolve, reject) => {
    timeoutID = setTimeout(() => reject(_makeTimeoutMessage(timeout, isHook, doneCallback)), timeout);

    // If this fn accepts `done` callback we return a promise that fulfills as
    // soon as `done` called.
    if (doneCallback) {
      let returnedValue = undefined;
      const done = reason => {
        // We need to keep a stack here before the promise tick
        const errorAtDone = new _jestUtil.ErrorWithStack(undefined, done);
        if (!completed && testOrHook.seenDone) {
          errorAtDone.message = 'Expected done to be called once, but it was called multiple times.';
          if (reason) {
            errorAtDone.message += ` Reason: ${(0, _prettyFormat.format)(reason, {
              maxDepth: 3
            })}`;
          }
          reject(errorAtDone);
          throw errorAtDone;
        } else {
          testOrHook.seenDone = true;
        }

        // Use `Promise.resolve` to allow the event loop to go a single tick in case `done` is called synchronously
        Promise.resolve().then(() => {
          if (returnedValue !== undefined) {
            asyncError.message = (0, _dedent.default)`
              Test functions cannot both take a 'done' callback and return something. Either use a 'done' callback, or return a promise.
              Returned value: ${(0, _prettyFormat.format)(returnedValue, {
              maxDepth: 3
            })}
            `;
            return reject(asyncError);
          }
          let errorAsErrorObject;
          if (checkIsError(reason)) {
            errorAsErrorObject = reason;
          } else {
            errorAsErrorObject = errorAtDone;
            errorAtDone.message = `Failed: ${(0, _prettyFormat.format)(reason, {
              maxDepth: 3
            })}`;
          }

          // Consider always throwing, regardless if `reason` is set or not
          if (completed && reason) {
            errorAsErrorObject.message = `Caught error after test environment was torn down\n\n${errorAsErrorObject.message}`;
            throw errorAsErrorObject;
          }
          return reason ? reject(errorAsErrorObject) : resolve();
        });
      };
      returnedValue = fn.call(testContext, done);
      return;
    }
    let returnedValue;
    if (isGeneratorFunction(fn)) {
      returnedValue = _co.default.wrap(fn).call({});
    } else {
      try {
        returnedValue = fn.call(testContext);
      } catch (error) {
        reject(error);
        return;
      }
    }
    if ((0, _jestUtil.isPromise)(returnedValue)) {
      returnedValue.then(() => resolve(), reject);
      return;
    }
    if (!isHook && returnedValue !== undefined) {
      reject(new Error((0, _dedent.default)`
            test functions can only return Promise or undefined.
            Returned value: ${(0, _prettyFormat.format)(returnedValue, {
        maxDepth: 3
      })}
          `));
      return;
    }

    // Otherwise this test is synchronous, and if it didn't throw it means
    // it passed.
    resolve();
  }).finally(() => {
    completed = true;
    // If timeout is not cleared/unrefed the node process won't exit until
    // it's resolved.
    timeoutID.unref?.();
    clearTimeout(timeoutID);
  });
};
exports.callAsyncCircusFn = callAsyncCircusFn;
const getTestDuration = test => {
  const {
    startedAt
  } = test;
  return typeof startedAt === 'number' ? jestNow() - startedAt : null;
};
exports.getTestDuration = getTestDuration;
const makeRunResult = (describeBlock, unhandledErrors) => ({
  testResults: makeTestResults(describeBlock),
  unhandledErrors: unhandledErrors.map(_getError).map(getErrorStack)
});
exports.makeRunResult = makeRunResult;
const getTestNamesPath = test => {
  const titles = [];
  let parent = test;
  do {
    titles.unshift(parent.name);
  } while (parent = parent.parent);
  return titles;
};
const makeSingleTestResult = test => {
  const {
    includeTestLocationInResult
  } = (0, _state.getState)();
  const {
    status
  } = test;
  (0, _jestUtil.invariant)(status, 'Status should be present after tests are run.');
  const testPath = getTestNamesPath(test);
  let location = null;
  if (includeTestLocationInResult) {
    const stackLines = test.asyncError.stack.split('\n');
    const stackLine = stackLines[1];
    let parsedLine = stackUtils.parseLine(stackLine);
    if (parsedLine?.file?.startsWith(jestEachBuildDir)) {
      const stackLine = stackLines[2];
      parsedLine = stackUtils.parseLine(stackLine);
    }
    if (parsedLine && typeof parsedLine.column === 'number' && typeof parsedLine.line === 'number') {
      location = {
        column: parsedLine.column,
        line: parsedLine.line
      };
    }
  }
  const errorsDetailed = test.errors.map(_getError);
  return {
    duration: test.duration,
    errors: errorsDetailed.map(getErrorStack),
    errorsDetailed,
    failing: test.failing,
    invocations: test.invocations,
    location,
    numPassingAsserts: test.numPassingAsserts,
    retryReasons: test.retryReasons.map(_getError).map(getErrorStack),
    startedAt: test.startedAt,
    status,
    testPath: [...testPath]
  };
};
exports.makeSingleTestResult = makeSingleTestResult;
const makeTestResults = describeBlock => {
  const testResults = [];
  const stack = [[describeBlock, 0]];
  while (stack.length > 0) {
    const [currentBlock, childIndex] = stack.pop();
    for (let i = childIndex; i < currentBlock.children.length; i++) {
      const child = currentBlock.children[i];
      if (child.type === 'describeBlock') {
        stack.push([currentBlock, i + 1], [child, 0]);
        break;
      }
      if (child.type === 'test') {
        testResults.push(makeSingleTestResult(child));
      }
    }
  }
  return testResults;
};

// Return a string that identifies the test (concat of parent describe block
// names + test title)
const getTestID = test => {
  const testNamesPath = getTestNamesPath(test);
  testNamesPath.shift(); // remove TOP_DESCRIBE_BLOCK_NAME
  return testNamesPath.join(' ');
};
exports.getTestID = getTestID;
const _getError = errors => {
  let error;
  let asyncError;
  if (Array.isArray(errors)) {
    error = errors[0];
    asyncError = errors[1];
  } else {
    error = errors;
    // eslint-disable-next-line unicorn/error-message
    asyncError = new Error();
  }
  if (error && (typeof error.stack === 'string' || error.message)) {
    return error;
  }
  if (asyncError) {
    asyncError.message = `thrown: ${(0, _prettyFormat.format)(error, {
      maxDepth: 3
    })}`;
    return asyncError;
  }
  return new Error(`thrown: ${(0, _prettyFormat.format)(error, {
    maxDepth: 3
  })}`);
};
const isErrorOrStackWithCause = errorOrStack => typeof errorOrStack !== 'string' && 'cause' in errorOrStack && (typeof errorOrStack.cause === 'string' || (0, _jestUtil.isError)(errorOrStack.cause) || errorOrStack.cause instanceof Error);
const formatErrorStackWithCause = (error, seen) => {
  const stack = typeof error.stack === 'string' && error.stack !== '' ? error.stack : error.message;
  if (!isErrorOrStackWithCause(error)) {
    return stack;
  }
  let cause;
  if (typeof error.cause === 'string') {
    cause = error.cause;
  } else if (seen.has(error.cause)) {
    cause = '[Circular cause]';
  } else {
    seen.add(error);
    cause = formatErrorStackWithCause(error.cause, seen);
  }
  return `${stack}\n\n[cause]: ${cause}`;
};
const getErrorStack = error => formatErrorStackWithCause(error, new Set());
const addErrorToEachTestUnderDescribe = (describeBlock, error, asyncError) => {
  for (const child of describeBlock.children) {
    switch (child.type) {
      case 'describeBlock':
        addErrorToEachTestUnderDescribe(child, error, asyncError);
        break;
      case 'test':
        child.errors.push([error, asyncError]);
        break;
    }
  }
};
exports.addErrorToEachTestUnderDescribe = addErrorToEachTestUnderDescribe;
const resolveTestCaseStartInfo = testNamesPath => {
  const ancestorTitles = testNamesPath.filter(name => name !== _state.ROOT_DESCRIBE_BLOCK_NAME);
  const fullName = ancestorTitles.join(' ');
  const title = testNamesPath.at(-1);
  // remove title
  ancestorTitles.pop();
  return {
    ancestorTitles,
    fullName,
    title
  };
};
const parseSingleTestResult = testResult => {
  let status;
  if (testResult.status === 'skip') {
    status = 'pending';
  } else if (testResult.status === 'todo') {
    status = 'todo';
  } else if (testResult.errors.length > 0) {
    status = 'failed';
  } else {
    status = 'passed';
  }
  const {
    ancestorTitles,
    fullName,
    title
  } = resolveTestCaseStartInfo(testResult.testPath);
  return {
    ancestorTitles,
    duration: testResult.duration,
    failing: testResult.failing,
    failureDetails: testResult.errorsDetailed,
    failureMessages: [...testResult.errors],
    fullName,
    invocations: testResult.invocations,
    location: testResult.location,
    numPassingAsserts: testResult.numPassingAsserts,
    retryReasons: [...testResult.retryReasons],
    startedAt: testResult.startedAt,
    status,
    title
  };
};
exports.parseSingleTestResult = parseSingleTestResult;
const createTestCaseStartInfo = test => {
  const testPath = getTestNamesPath(test);
  const {
    ancestorTitles,
    fullName,
    title
  } = resolveTestCaseStartInfo(testPath);
  return {
    ancestorTitles,
    fullName,
    mode: test.mode,
    startedAt: test.startedAt,
    title
  };
};
exports.createTestCaseStartInfo = createTestCaseStartInfo;

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
exports.runAndTransformResultsToJestFormat = exports.initialize = exports.eventHandler = exports.collectTestsWithoutRunning = void 0;
var _expect = require("@jest/expect");
var _testResult = require("@jest/test-result");
var _jestMessageUtil = require("jest-message-util");
var _jestSnapshot = require("jest-snapshot");
var _ = _interopRequireDefault(__webpack_require__("./src/index.ts"));
var _run = _interopRequireDefault(__webpack_require__("./src/run.ts"));
var _state = __webpack_require__("./src/state.ts");
var _testCaseReportHandler = _interopRequireDefault(__webpack_require__("./src/testCaseReportHandler.ts"));
var _unhandledRejectionHandler = __webpack_require__("./src/unhandledRejectionHandler.ts");
var _utils = __webpack_require__("./src/utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const initialize = async ({
  config,
  environment,
  runtime,
  globalConfig,
  localRequire,
  parentProcess,
  sendMessageToJest,
  setGlobalsForRuntime,
  testPath
}) => {
  if (globalConfig.testTimeout) {
    (0, _state.getState)().testTimeout = globalConfig.testTimeout;
  }
  (0, _state.getState)().maxConcurrency = globalConfig.maxConcurrency;
  (0, _state.getState)().randomize = globalConfig.randomize;
  (0, _state.getState)().seed = globalConfig.seed;

  // @ts-expect-error: missing `concurrent` which is added later
  const globalsObject = {
    ..._.default,
    fdescribe: _.default.describe.only,
    fit: _.default.it.only,
    xdescribe: _.default.describe.skip,
    xit: _.default.it.skip,
    xtest: _.default.it.skip
  };
  (0, _state.addEventHandler)(eventHandler);
  if (environment.handleTestEvent) {
    (0, _state.addEventHandler)(environment.handleTestEvent.bind(environment));
  }
  _expect.jestExpect.setState({
    expand: globalConfig.expand
  });
  const runtimeGlobals = {
    ...globalsObject,
    expect: _expect.jestExpect
  };
  setGlobalsForRuntime(runtimeGlobals);
  if (config.injectGlobals) {
    Object.assign(environment.global, runtimeGlobals);
  }
  await (0, _state.dispatch)({
    name: 'setup',
    parentProcess,
    runtimeGlobals,
    testNamePattern: globalConfig.testNamePattern
  });
  if (config.testLocationInResults) {
    await (0, _state.dispatch)({
      name: 'include_test_location_in_result'
    });
  }

  // Jest tests snapshotSerializers in order preceding built-in serializers.
  // Therefore, add in reverse because the last added is the first tested.
  for (const path of [...config.snapshotSerializers].reverse()) (0, _jestSnapshot.addSerializer)(localRequire(path));
  const snapshotResolver = await (0, _jestSnapshot.buildSnapshotResolver)(config, localRequire);
  const snapshotPath = snapshotResolver.resolveSnapshotPath(testPath);
  const snapshotState = new _jestSnapshot.SnapshotState(snapshotPath, {
    expand: globalConfig.expand,
    prettierPath: config.prettierPath,
    rootDir: config.rootDir,
    snapshotFormat: config.snapshotFormat,
    updateSnapshot: globalConfig.updateSnapshot
  });
  _expect.jestExpect.setState({
    snapshotState,
    testPath
  });
  (0, _state.addEventHandler)(handleSnapshotStateAfterRetry(snapshotState));
  if (sendMessageToJest) {
    (0, _state.addEventHandler)((0, _testCaseReportHandler.default)(testPath, sendMessageToJest));
  }
  (0, _state.addEventHandler)((0, _unhandledRejectionHandler.unhandledRejectionHandler)(runtime, globalConfig.waitForUnhandledRejections));

  // Return it back to the outer scope (test runner outside the VM).
  return {
    globals: globalsObject,
    snapshotState
  };
};
exports.initialize = initialize;
const collectTestsWithoutRunning = async ({
  config,
  testPath
}) => {
  const {
    rootDescribeBlock,
    testNamePattern
  } = (0, _state.getState)();
  const assertionResults = [];
  const walk = (block, ancestors) => {
    for (const child of block.children) {
      if (child.type === 'describeBlock') {
        walk(child, [...ancestors, child.name]);
        continue;
      }
      if (testNamePattern && !testNamePattern.test((0, _utils.getTestID)(child))) {
        continue;
      }
      const title = child.name;
      assertionResults.push({
        ancestorTitles: [...ancestors],
        duration: null,
        failing: false,
        failureDetails: [],
        failureMessages: [],
        fullName: [...ancestors, title].join(' '),
        invocations: 0,
        location: null,
        numPassingAsserts: 0,
        retryReasons: [],
        startAt: null,
        status: 'pending',
        title
      });
    }
  };
  walk(rootDescribeBlock, []);
  await (0, _state.dispatch)({
    name: 'teardown'
  });
  return {
    ...(0, _testResult.createEmptyTestResult)(),
    displayName: config.displayName,
    numPendingTests: assertionResults.length,
    testFilePath: testPath,
    testResults: assertionResults
  };
};
exports.collectTestsWithoutRunning = collectTestsWithoutRunning;
const runAndTransformResultsToJestFormat = async ({
  config,
  globalConfig,
  setupAfterEnvPerfStats,
  testPath
}) => {
  const runResult = await (0, _run.default)();
  let numFailingTests = 0;
  let numPassingTests = 0;
  let numPendingTests = 0;
  let numTodoTests = 0;
  const assertionResults = runResult.testResults.map(testResult => {
    let status;
    if (testResult.status === 'skip') {
      status = 'pending';
      numPendingTests += 1;
    } else if (testResult.status === 'todo') {
      status = 'todo';
      numTodoTests += 1;
    } else if (testResult.errors.length > 0) {
      status = 'failed';
      numFailingTests += 1;
    } else {
      status = 'passed';
      numPassingTests += 1;
    }
    const ancestorTitles = testResult.testPath.filter(name => name !== _state.ROOT_DESCRIBE_BLOCK_NAME);
    const title = ancestorTitles.pop();
    return {
      ancestorTitles,
      duration: testResult.duration,
      failing: testResult.failing,
      failureDetails: testResult.errorsDetailed,
      failureMessages: testResult.errors,
      fullName: title ? [...ancestorTitles, title].join(' ') : ancestorTitles.join(' '),
      invocations: testResult.invocations,
      location: testResult.location,
      numPassingAsserts: testResult.numPassingAsserts,
      retryReasons: testResult.retryReasons,
      startAt: testResult.startedAt,
      status,
      title: testResult.testPath.at(-1)
    };
  });
  let failureMessage = (0, _jestMessageUtil.formatResultsErrors)(assertionResults, config, globalConfig, testPath);
  let testExecError;
  if (runResult.unhandledErrors.length > 0) {
    testExecError = {
      message: '',
      stack: runResult.unhandledErrors.join('\n')
    };
    failureMessage = `${failureMessage || ''}\n\n${runResult.unhandledErrors.map(err => (0, _jestMessageUtil.formatExecError)(err, config, globalConfig)).join('\n')}`;
  }
  await (0, _state.dispatch)({
    name: 'teardown'
  });
  const emptyTestResult = (0, _testResult.createEmptyTestResult)();
  return {
    ...emptyTestResult,
    console: undefined,
    displayName: config.displayName,
    failureMessage,
    numFailingTests,
    numPassingTests,
    numPendingTests,
    numTodoTests,
    perfStats: {
      ...emptyTestResult.perfStats,
      ...setupAfterEnvPerfStats
    },
    testExecError,
    testFilePath: testPath,
    testResults: assertionResults
  };
};
exports.runAndTransformResultsToJestFormat = runAndTransformResultsToJestFormat;
const handleSnapshotStateAfterRetry = snapshotState => event => {
  switch (event.name) {
    case 'test_retry':
      {
        // Clear any snapshot data that occurred in previous test run
        snapshotState.clear();
      }
  }
};

// Exported for direct access from unit tests.
const eventHandler = async event => {
  switch (event.name) {
    case 'test_start':
      {
        _expect.jestExpect.setState({
          currentTestName: (0, _utils.getTestID)(event.test),
          testFailing: event.test.failing
        });
        break;
      }
    case 'test_done':
      {
        event.test.numPassingAsserts = _expect.jestExpect.getState().numPassingAsserts;
        _addSuppressedErrors(event.test);
        _addExpectedAssertionErrors(event.test);
        break;
      }
  }
};
exports.eventHandler = eventHandler;
const _addExpectedAssertionErrors = test => {
  const {
    isExpectingAssertions
  } = _expect.jestExpect.getState();
  const failures = _expect.jestExpect.extractExpectedAssertionsErrors();
  if (isExpectingAssertions && test.errors.length > 0) {
    // Only show errors from `expect.hasAssertions()` when no other failure has happened.
    return;
  }
  test.errors.push(...failures.map(failure => failure.error));
};

// Get suppressed errors from ``jest-matchers`` that weren't throw during
// test execution and add them to the test result, potentially failing
// a passing test.
const _addSuppressedErrors = test => {
  const {
    suppressedErrors
  } = _expect.jestExpect.getState();
  _expect.jestExpect.setState({
    suppressedErrors: []
  });
  if (suppressedErrors.length > 0) {
    test.errors.push(...suppressedErrors);
  }
};
})();

module.exports = __webpack_exports__;
/******/ })()
;