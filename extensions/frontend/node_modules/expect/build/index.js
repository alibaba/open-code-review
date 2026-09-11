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

/***/ "./src/asymmetricMatchers.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.closeTo = exports.arrayOf = exports.arrayNotContaining = exports.arrayContaining = exports.anything = exports.any = exports.AsymmetricMatcher = void 0;
exports.hasProperty = hasProperty;
exports.stringNotMatching = exports.stringNotContaining = exports.stringMatching = exports.stringContaining = exports.objectNotContaining = exports.objectContaining = exports.notCloseTo = exports.notArrayOf = void 0;
var _expectUtils = require("@jest/expect-utils");
var matcherUtils = _interopRequireWildcard(require("jest-matcher-utils"));
var _jestUtil = require("jest-util");
var _jestMatchersObject = __webpack_require__("./src/jestMatchersObject.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */
const functionToString = Function.prototype.toString;
function fnNameFor(func) {
  if (func.name) {
    return func.name;
  }
  const matches = functionToString.call(func).match(/^(?:async)?\s*function\s*\*?\s*([\w$]+)\s*\(/);
  return matches ? matches[1] : '<anonymous>';
}
const utils = Object.freeze({
  ...matcherUtils,
  iterableEquality: _expectUtils.iterableEquality,
  subsetEquality: _expectUtils.subsetEquality
});
function hasProperty(obj, property) {
  if (!obj) {
    return false;
  }
  if (Object.prototype.hasOwnProperty.call(obj, property)) {
    return true;
  }
  return hasProperty(Object.getPrototypeOf(obj), property);
}
class AsymmetricMatcher {
  $$typeof = Symbol.for('jest.asymmetricMatcher');
  constructor(sample, inverse = false) {
    this.sample = sample;
    this.inverse = inverse;
  }
  getMatcherContext() {
    return {
      customTesters: (0, _jestMatchersObject.getCustomEqualityTesters)(),
      // eslint-disable-next-line @typescript-eslint/no-empty-function
      dontThrow: () => {},
      ...(0, _jestMatchersObject.getState)(),
      equals: _expectUtils.equals,
      isNot: this.inverse,
      utils
    };
  }
}
exports.AsymmetricMatcher = AsymmetricMatcher;
class Any extends AsymmetricMatcher {
  constructor(sample) {
    if (sample === undefined) {
      throw new TypeError('any() expects to be passed a constructor function. ' + 'Please pass one or use anything() to match any object.');
    }
    super(sample);
  }
  asymmetricMatch(other) {
    if (this.sample === String) {
      // eslint-disable-next-line unicorn/no-instanceof-builtins
      return typeof other === 'string' || other instanceof String;
    }
    if (this.sample === Number) {
      // eslint-disable-next-line unicorn/no-instanceof-builtins
      return typeof other === 'number' || other instanceof Number;
    }
    if (this.sample === Function) {
      // eslint-disable-next-line unicorn/no-instanceof-builtins
      return typeof other === 'function' || other instanceof Function;
    }
    if (this.sample === Boolean) {
      // eslint-disable-next-line unicorn/no-instanceof-builtins
      return typeof other === 'boolean' || other instanceof Boolean;
    }
    if (this.sample === BigInt) {
      // eslint-disable-next-line unicorn/no-instanceof-builtins
      return typeof other === 'bigint' || other instanceof BigInt;
    }
    if (this.sample === Symbol) {
      // eslint-disable-next-line unicorn/no-instanceof-builtins
      return typeof other === 'symbol' || other instanceof Symbol;
    }
    if (this.sample === Object) {
      return typeof other === 'object';
    }
    if (this.sample === Array) {
      return Array.isArray(other);
    }
    return other instanceof this.sample;
  }
  toString() {
    return 'Any';
  }
  getExpectedType() {
    if (this.sample === String) {
      return 'string';
    }
    if (this.sample === Number) {
      return 'number';
    }
    if (this.sample === Function) {
      return 'function';
    }
    if (this.sample === Object) {
      return 'object';
    }
    if (this.sample === Boolean) {
      return 'boolean';
    }
    if (this.sample === Array) {
      return 'array';
    }
    return fnNameFor(this.sample);
  }
  toAsymmetricMatcher() {
    return `Any<${fnNameFor(this.sample)}>`;
  }
}
class Anything extends AsymmetricMatcher {
  asymmetricMatch(other) {
    return other != null;
  }
  toString() {
    return 'Anything';
  }

  // No getExpectedType method, because it matches either null or undefined.

  toAsymmetricMatcher() {
    return 'Anything';
  }
}
class ArrayContaining extends AsymmetricMatcher {
  constructor(sample, inverse = false) {
    super(sample, inverse);
  }
  asymmetricMatch(other) {
    if (!Array.isArray(this.sample)) {
      throw new TypeError(`You must provide an array to ${this.toString()}, not '${typeof this.sample}'.`);
    }
    const matcherContext = this.getMatcherContext();
    const result = this.sample.length === 0 || Array.isArray(other) && this.sample.every(item => other.some(another => (0, _expectUtils.equals)(item, another, matcherContext.customTesters)));
    return this.inverse ? !result : result;
  }
  toString() {
    return `Array${this.inverse ? 'Not' : ''}Containing`;
  }
  getExpectedType() {
    return 'array';
  }
}
class ArrayOf extends AsymmetricMatcher {
  asymmetricMatch(other) {
    const matcherContext = this.getMatcherContext();
    const result = Array.isArray(other) && other.every(item => (0, _expectUtils.equals)(this.sample, item, matcherContext.customTesters));
    return this.inverse ? !result : result;
  }
  toString() {
    return `${this.inverse ? 'Not' : ''}ArrayOf`;
  }
  getExpectedType() {
    return 'array';
  }
}
class ObjectContaining extends AsymmetricMatcher {
  constructor(sample, inverse = false) {
    super(sample, inverse);
  }
  asymmetricMatch(other) {
    // Ensures that the argument passed to the objectContaining method is an object
    if (typeof this.sample !== 'object') {
      throw new TypeError(`You must provide an object to ${this.toString()}, not '${typeof this.sample}'.`);
    }

    // Ensures that the argument passed to the expect function is an object
    // This is necessary to avoid matching of non-object values
    // Arrays are a special type of object, but having a valid match with a standard object
    // does not make sense, hence we do a simple array check
    if (typeof other !== 'object' || Array.isArray(other)) {
      return false;
    }
    let result = true;
    const matcherContext = this.getMatcherContext();
    const objectKeys = (0, _expectUtils.getObjectKeys)(this.sample);
    for (const key of objectKeys) {
      if (!hasProperty(other, key) || !(0, _expectUtils.equals)(this.sample[key], other[key], matcherContext.customTesters)) {
        result = false;
        break;
      }
    }
    return this.inverse ? !result : result;
  }
  toString() {
    return `Object${this.inverse ? 'Not' : ''}Containing`;
  }
  getExpectedType() {
    return 'object';
  }
}
class StringContaining extends AsymmetricMatcher {
  constructor(sample, inverse = false) {
    if (!(0, _expectUtils.isA)('String', sample)) {
      throw new Error('Expected is not a string');
    }
    super(sample, inverse);
  }
  asymmetricMatch(other) {
    const result = (0, _expectUtils.isA)('String', other) && other.includes(this.sample);
    return this.inverse ? !result : result;
  }
  toString() {
    return `String${this.inverse ? 'Not' : ''}Containing`;
  }
  getExpectedType() {
    return 'string';
  }
}
class StringMatching extends AsymmetricMatcher {
  constructor(sample, inverse = false) {
    if (!(0, _expectUtils.isA)('String', sample) && !(0, _expectUtils.isA)('RegExp', sample)) {
      throw new Error('Expected is not a String or a RegExp');
    }
    super(new RegExp(sample), inverse);
  }
  asymmetricMatch(other) {
    const result = (0, _expectUtils.isA)('String', other) && this.sample.test(other);
    return this.inverse ? !result : result;
  }
  toString() {
    return `String${this.inverse ? 'Not' : ''}Matching`;
  }
  getExpectedType() {
    return 'string';
  }
}
class CloseTo extends AsymmetricMatcher {
  precision;
  constructor(sample, precision = 2, inverse = false) {
    if (!(0, _expectUtils.isA)('Number', sample)) {
      throw new Error('Expected is not a Number');
    }
    if (!(0, _expectUtils.isA)('Number', precision)) {
      throw new Error('Precision is not a Number');
    }
    super(sample);
    this.inverse = inverse;
    this.precision = precision;
  }
  asymmetricMatch(other) {
    if (!(0, _expectUtils.isA)('Number', other)) {
      return false;
    }
    let result = false;
    if (other === Number.POSITIVE_INFINITY && this.sample === Number.POSITIVE_INFINITY) {
      result = true; // Infinity - Infinity is NaN
    } else if (other === Number.NEGATIVE_INFINITY && this.sample === Number.NEGATIVE_INFINITY) {
      result = true; // -Infinity - -Infinity is NaN
    } else {
      result = Math.abs(this.sample - other) < Math.pow(10, -this.precision) / 2;
    }
    return this.inverse ? !result : result;
  }
  toString() {
    return `Number${this.inverse ? 'Not' : ''}CloseTo`;
  }
  getExpectedType() {
    return 'number';
  }
  toAsymmetricMatcher() {
    return [this.toString(), this.sample, `(${(0, _jestUtil.pluralize)('digit', this.precision)})`].join(' ');
  }
}
const any = expectedObject => new Any(expectedObject);
exports.any = any;
const anything = () => new Anything();
exports.anything = anything;
const arrayContaining = sample => new ArrayContaining(sample);
exports.arrayContaining = arrayContaining;
const arrayNotContaining = sample => new ArrayContaining(sample, true);
exports.arrayNotContaining = arrayNotContaining;
const arrayOf = sample => new ArrayOf(sample);
exports.arrayOf = arrayOf;
const notArrayOf = sample => new ArrayOf(sample, true);
exports.notArrayOf = notArrayOf;
const objectContaining = sample => new ObjectContaining(sample);
exports.objectContaining = objectContaining;
const objectNotContaining = sample => new ObjectContaining(sample, true);
exports.objectNotContaining = objectNotContaining;
const stringContaining = expected => new StringContaining(expected);
exports.stringContaining = stringContaining;
const stringNotContaining = expected => new StringContaining(expected, true);
exports.stringNotContaining = stringNotContaining;
const stringMatching = expected => new StringMatching(expected);
exports.stringMatching = stringMatching;
const stringNotMatching = expected => new StringMatching(expected, true);
exports.stringNotMatching = stringNotMatching;
const closeTo = (expected, precision) => new CloseTo(expected, precision);
exports.closeTo = closeTo;
const notCloseTo = (expected, precision) => new CloseTo(expected, precision, true);
exports.notCloseTo = notCloseTo;

/***/ },

/***/ "./src/extractExpectedAssertionsErrors.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _jestMatcherUtils = require("jest-matcher-utils");
var _jestMatchersObject = __webpack_require__("./src/jestMatchersObject.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const resetAssertionsLocalState = () => {
  (0, _jestMatchersObject.setState)({
    assertionCalls: 0,
    expectedAssertionsNumber: null,
    isExpectingAssertions: false,
    numPassingAsserts: 0
  });
};

// Create and format all errors related to the mismatched number of `expect`
// calls and reset the matcher's state.
const extractExpectedAssertionsErrors = () => {
  const result = [];
  const {
    assertionCalls,
    expectedAssertionsNumber,
    expectedAssertionsNumberError,
    isExpectingAssertions,
    isExpectingAssertionsError
  } = (0, _jestMatchersObject.getState)();
  resetAssertionsLocalState();
  if (typeof expectedAssertionsNumber === 'number' && assertionCalls !== expectedAssertionsNumber) {
    const numOfAssertionsExpected = (0, _jestMatcherUtils.EXPECTED_COLOR)((0, _jestMatcherUtils.pluralize)('assertion', expectedAssertionsNumber));
    expectedAssertionsNumberError.message = `${(0, _jestMatcherUtils.matcherHint)('.assertions', '', expectedAssertionsNumber.toString(), {
      isDirectExpectCall: true
    })}\n\n` + `Expected ${numOfAssertionsExpected} to be called but received ${(0, _jestMatcherUtils.RECEIVED_COLOR)((0, _jestMatcherUtils.pluralize)('assertion call', assertionCalls || 0))}.`;
    result.push({
      actual: assertionCalls.toString(),
      error: expectedAssertionsNumberError,
      expected: expectedAssertionsNumber.toString()
    });
  }
  if (isExpectingAssertions && assertionCalls === 0) {
    const expected = (0, _jestMatcherUtils.EXPECTED_COLOR)('at least one assertion');
    const received = (0, _jestMatcherUtils.RECEIVED_COLOR)('received none');
    isExpectingAssertionsError.message = `${(0, _jestMatcherUtils.matcherHint)('.hasAssertions', '', '', {
      isDirectExpectCall: true
    })}\n\nExpected ${expected} to be called but ${received}.`;
    result.push({
      actual: 'none',
      error: isExpectingAssertionsError,
      expected: 'at least one'
    });
  }
  return result;
};
var _default = exports["default"] = extractExpectedAssertionsErrors;

/***/ },

/***/ "./src/jestMatchersObject.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.setState = exports.setMatchers = exports.getState = exports.getMatchers = exports.getCustomEqualityTesters = exports.addCustomEqualityTesters = exports.INTERNAL_MATCHER_FLAG = void 0;
var _getType = require("@jest/get-type");
var _asymmetricMatchers = __webpack_require__("./src/asymmetricMatchers.ts");
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */
// Global matchers object holds the list of available matchers and
// the state, that can hold matcher specific values that change over time.
const JEST_MATCHERS_OBJECT = Symbol.for('$$jest-matchers-object');

// Notes a built-in/internal Jest matcher.
// Jest may override the stack trace of Errors thrown by internal matchers.
const INTERNAL_MATCHER_FLAG = exports.INTERNAL_MATCHER_FLAG = Symbol.for('$$jest-internal-matcher');
if (!Object.prototype.hasOwnProperty.call(globalThis, JEST_MATCHERS_OBJECT)) {
  const defaultState = {
    assertionCalls: 0,
    expectedAssertionsNumber: null,
    isExpectingAssertions: false,
    numPassingAsserts: 0,
    suppressedErrors: [] // errors that are not thrown immediately.
  };
  Object.defineProperty(globalThis, JEST_MATCHERS_OBJECT, {
    value: {
      customEqualityTesters: [],
      matchers: Object.create(null),
      state: defaultState
    }
  });
}
const getState = () => globalThis[JEST_MATCHERS_OBJECT].state;
exports.getState = getState;
const setState = state => {
  Object.assign(globalThis[JEST_MATCHERS_OBJECT].state, state);
};
exports.setState = setState;
const getMatchers = () => globalThis[JEST_MATCHERS_OBJECT].matchers;
exports.getMatchers = getMatchers;
const setMatchers = (matchers, isInternal, expect) => {
  for (const key of Object.keys(matchers)) {
    const matcher = matchers[key];
    if (typeof matcher !== 'function') {
      throw new TypeError(`expect.extend: \`${key}\` is not a valid matcher. Must be a function, is "${(0, _getType.getType)(matcher)}"`);
    }
    Object.defineProperty(matcher, INTERNAL_MATCHER_FLAG, {
      value: isInternal
    });
    if (!isInternal) {
      // expect is defined

      class CustomMatcher extends _asymmetricMatchers.AsymmetricMatcher {
        constructor(inverse = false, ...sample) {
          super(sample, inverse);
        }
        asymmetricMatch(other) {
          const {
            pass
          } = matcher.call(this.getMatcherContext(), other, ...this.sample);
          return this.inverse ? !pass : pass;
        }
        toString() {
          return `${this.inverse ? 'not.' : ''}${key}`;
        }
        getExpectedType() {
          return 'any';
        }
        toAsymmetricMatcher() {
          return `${this.toString()}<${this.sample.map(String).join(', ')}>`;
        }
      }
      Object.defineProperty(expect, key, {
        configurable: true,
        enumerable: true,
        value: (...sample) => new CustomMatcher(false, ...sample),
        writable: true
      });
      Object.defineProperty(expect.not, key, {
        configurable: true,
        enumerable: true,
        value: (...sample) => new CustomMatcher(true, ...sample),
        writable: true
      });
    }
  }
  Object.assign(globalThis[JEST_MATCHERS_OBJECT].matchers, matchers);
};
exports.setMatchers = setMatchers;
const getCustomEqualityTesters = () => globalThis[JEST_MATCHERS_OBJECT].customEqualityTesters;
exports.getCustomEqualityTesters = getCustomEqualityTesters;
const addCustomEqualityTesters = newTesters => {
  if (!Array.isArray(newTesters)) {
    throw new TypeError(`expect.customEqualityTesters: Must be set to an array of Testers. Was given "${(0, _getType.getType)(newTesters)}"`);
  }
  globalThis[JEST_MATCHERS_OBJECT].customEqualityTesters.push(...newTesters);
};
exports.addCustomEqualityTesters = addCustomEqualityTesters;

/***/ },

/***/ "./src/matchers.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _expectUtils = require("@jest/expect-utils");
var _getType = require("@jest/get-type");
var _jestMatcherUtils = require("jest-matcher-utils");
var _print = __webpack_require__("./src/print.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

// Omit colon and one or more spaces, so can call getLabelPrinter.
const EXPECTED_LABEL = 'Expected';
const RECEIVED_LABEL = 'Received';
const EXPECTED_VALUE_LABEL = 'Expected value';
const RECEIVED_VALUE_LABEL = 'Received value';

// The optional property of matcher context is true if undefined.
const isExpand = expand => expand !== false;
const toStrictEqualTesters = [_expectUtils.iterableEquality, _expectUtils.typeEquality, _expectUtils.sparseArrayEquality, _expectUtils.arrayBufferEquality];
const matchers = {
  toBe(received, expected) {
    const matcherName = 'toBe';
    const options = {
      comment: 'Object.is equality',
      isNot: this.isNot,
      promise: this.promise
    };
    const pass = Object.is(received, expected);
    const message = pass ? () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}` : () => {
      const expectedType = (0, _getType.getType)(expected);
      let deepEqualityName = null;
      if (expectedType !== 'map' && expectedType !== 'set') {
        // If deep equality passes when referential identity fails,
        // but exclude map and set until review of their equality logic.
        if ((0, _expectUtils.equals)(received, expected, [...this.customTesters, ...toStrictEqualTesters], true)) {
          deepEqualityName = 'toStrictEqual';
        } else if ((0, _expectUtils.equals)(received, expected, [...this.customTesters, _expectUtils.iterableEquality])) {
          deepEqualityName = 'toEqual';
        }
      }
      return (
        // eslint-disable-next-line prefer-template
        (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (deepEqualityName === null ? '' : `${(0, _jestMatcherUtils.DIM_COLOR)(`If it should pass with deep equality, replace "${matcherName}" with "${deepEqualityName}"`)}\n\n`) + (0, _jestMatcherUtils.printDiffOrStringify)(expected, received, EXPECTED_LABEL, RECEIVED_LABEL, isExpand(this.expand))
      );
    };

    // Passing the actual and expected objects so that a custom reporter
    // could access them, for example in order to display a custom visual diff,
    // or create a different error message
    return {
      actual: received,
      expected,
      message,
      name: matcherName,
      pass
    };
  },
  toBeCloseTo(received, expected, precision = 2) {
    const matcherName = 'toBeCloseTo';
    const secondArgument = arguments.length === 3 ? 'precision' : undefined;
    const isNot = this.isNot;
    const options = {
      isNot,
      promise: this.promise,
      secondArgument,
      secondArgumentColor: arg => arg
    };
    if (typeof expected !== 'number') {
      throw new TypeError((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} value must be a number`, (0, _jestMatcherUtils.printWithType)('Expected', expected, _jestMatcherUtils.printExpected)));
    }
    if (typeof received !== 'number') {
      throw new TypeError((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be a number`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
    }
    let pass = false;
    let expectedDiff = 0;
    let receivedDiff = 0;
    if (received === Number.POSITIVE_INFINITY && expected === Number.POSITIVE_INFINITY) {
      pass = true; // Infinity - Infinity is NaN
    } else if (received === Number.NEGATIVE_INFINITY && expected === Number.NEGATIVE_INFINITY) {
      pass = true; // -Infinity - -Infinity is NaN
    } else {
      expectedDiff = Math.pow(10, -precision) / 2;
      receivedDiff = Math.abs(expected - received);
      pass = receivedDiff < expectedDiff;
    }
    const message = pass ? () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + (receivedDiff === 0 ? '' : `Received:     ${(0, _jestMatcherUtils.printReceived)(received)}\n` + `\n${(0, _print.printCloseTo)(receivedDiff, expectedDiff, precision, isNot)}`) : () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected: ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received: ${(0, _jestMatcherUtils.printReceived)(received)}\n` + '\n' + (0, _print.printCloseTo)(receivedDiff, expectedDiff, precision, isNot);
    return {
      message,
      pass
    };
  },
  toBeDefined(received, expected) {
    const matcherName = 'toBeDefined';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNoExpected)(expected, matcherName, options);
    const pass = received !== void 0;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + `Received: ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeFalsy(received, expected) {
    const matcherName = 'toBeFalsy';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNoExpected)(expected, matcherName, options);
    const pass = !received;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + `Received: ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeGreaterThan(received, expected) {
    const matcherName = 'toBeGreaterThan';
    const isNot = this.isNot;
    const options = {
      isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNumbers)(received, expected, matcherName, options);
    const pass = received > expected;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected:${isNot ? ' not' : ''} > ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received:${isNot ? '    ' : ''}   ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeGreaterThanOrEqual(received, expected) {
    const matcherName = 'toBeGreaterThanOrEqual';
    const isNot = this.isNot;
    const options = {
      isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNumbers)(received, expected, matcherName, options);
    const pass = received >= expected;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected:${isNot ? ' not' : ''} >= ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received:${isNot ? '    ' : ''}    ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeInstanceOf(received, expected) {
    const matcherName = 'toBeInstanceOf';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    if (typeof expected !== 'function') {
      throw new TypeError((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} value must be a function`, (0, _jestMatcherUtils.printWithType)('Expected', expected, _jestMatcherUtils.printExpected)));
    }
    const pass = received instanceof expected;
    const message = pass ? () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (0, _print.printExpectedConstructorNameNot)('Expected constructor', expected) + (typeof received.constructor === 'function' && received.constructor !== expected ? (0, _print.printReceivedConstructorNameNot)('Received constructor', received.constructor, expected) : '') : () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (0, _print.printExpectedConstructorName)('Expected constructor', expected) + ((0, _getType.isPrimitive)(received) || Object.getPrototypeOf(received) === null ? `\nReceived value has no prototype\nReceived value: ${(0, _jestMatcherUtils.printReceived)(received)}` : typeof received.constructor === 'function' ? (0, _print.printReceivedConstructorName)('Received constructor', received.constructor) : `\nReceived value: ${(0, _jestMatcherUtils.printReceived)(received)}`);
    return {
      message,
      pass
    };
  },
  toBeLessThan(received, expected) {
    const matcherName = 'toBeLessThan';
    const isNot = this.isNot;
    const options = {
      isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNumbers)(received, expected, matcherName, options);
    const pass = received < expected;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected:${isNot ? ' not' : ''} < ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received:${isNot ? '    ' : ''}   ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeLessThanOrEqual(received, expected) {
    const matcherName = 'toBeLessThanOrEqual';
    const isNot = this.isNot;
    const options = {
      isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNumbers)(received, expected, matcherName, options);
    const pass = received <= expected;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected:${isNot ? ' not' : ''} <= ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received:${isNot ? '    ' : ''}    ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeNaN(received, expected) {
    const matcherName = 'toBeNaN';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNoExpected)(expected, matcherName, options);
    const pass = Number.isNaN(received);
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + `Received: ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeNull(received, expected) {
    const matcherName = 'toBeNull';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNoExpected)(expected, matcherName, options);
    const pass = received === null;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + `Received: ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeTruthy(received, expected) {
    const matcherName = 'toBeTruthy';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNoExpected)(expected, matcherName, options);
    const pass = !!received;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + `Received: ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toBeUndefined(received, expected) {
    const matcherName = 'toBeUndefined';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    (0, _jestMatcherUtils.ensureNoExpected)(expected, matcherName, options);
    const pass = received === void 0;
    const message = () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + `Received: ${(0, _jestMatcherUtils.printReceived)(received)}`;
    return {
      message,
      pass
    };
  },
  toContain(received, expected) {
    const matcherName = 'toContain';
    const isNot = this.isNot;
    const options = {
      comment: 'indexOf',
      isNot,
      promise: this.promise
    };
    if (received == null) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must not be null nor undefined`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
    }
    if (typeof received === 'string') {
      const wrongTypeErrorMessage = `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} value must be a string if ${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value is a string`;
      if (typeof expected !== 'string') {
        throw new TypeError((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, received, String(expected), options), wrongTypeErrorMessage,
        // eslint-disable-next-line prefer-template
        (0, _jestMatcherUtils.printWithType)('Expected', expected, _jestMatcherUtils.printExpected) + '\n' + (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
      }
      const index = received.indexOf(String(expected));
      const pass = index !== -1;
      const message = () => {
        const labelExpected = `Expected ${typeof expected === 'string' ? 'substring' : 'value'}`;
        const labelReceived = 'Received string';
        const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(labelExpected, labelReceived);
        return (
          // eslint-disable-next-line prefer-template
          (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `${printLabel(labelExpected)}${isNot ? 'not ' : ''}${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `${printLabel(labelReceived)}${isNot ? '    ' : ''}${isNot ? (0, _print.printReceivedStringContainExpectedSubstring)(received, index, String(expected).length) : (0, _jestMatcherUtils.printReceived)(received)}`
        );
      };
      return {
        message,
        pass
      };
    }
    const indexable = [...received];
    const index = indexable.indexOf(expected);
    const pass = index !== -1;
    const message = () => {
      const labelExpected = 'Expected value';
      const labelReceived = `Received ${(0, _getType.getType)(received)}`;
      const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(labelExpected, labelReceived);
      return (
        // eslint-disable-next-line prefer-template
        (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `${printLabel(labelExpected)}${isNot ? 'not ' : ''}${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `${printLabel(labelReceived)}${isNot ? '    ' : ''}${isNot && Array.isArray(received) ? (0, _print.printReceivedArrayContainExpectedItem)(received, index) : (0, _jestMatcherUtils.printReceived)(received)}` + (!isNot && indexable.some(item => (0, _expectUtils.equals)(item, expected, [...this.customTesters, _expectUtils.iterableEquality])) ? `\n\n${_jestMatcherUtils.SUGGEST_TO_CONTAIN_EQUAL}` : '')
      );
    };
    return {
      message,
      pass
    };
  },
  toContainEqual(received, expected) {
    const matcherName = 'toContainEqual';
    const isNot = this.isNot;
    const options = {
      comment: 'deep equality',
      isNot,
      promise: this.promise
    };
    if (received == null) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must not be null nor undefined`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
    }
    const index = [...received].findIndex(item => (0, _expectUtils.equals)(item, expected, [...this.customTesters, _expectUtils.iterableEquality]));
    const pass = index !== -1;
    const message = () => {
      const labelExpected = 'Expected value';
      const labelReceived = `Received ${(0, _getType.getType)(received)}`;
      const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(labelExpected, labelReceived);
      return (
        // eslint-disable-next-line prefer-template
        (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `${printLabel(labelExpected)}${isNot ? 'not ' : ''}${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `${printLabel(labelReceived)}${isNot ? '    ' : ''}${isNot && Array.isArray(received) ? (0, _print.printReceivedArrayContainExpectedItem)(received, index) : (0, _jestMatcherUtils.printReceived)(received)}`
      );
    };
    return {
      message,
      pass
    };
  },
  toEqual(received, expected) {
    const matcherName = 'toEqual';
    const options = {
      comment: 'deep equality',
      isNot: this.isNot,
      promise: this.promise
    };
    const pass = (0, _expectUtils.equals)(received, expected, [...this.customTesters, _expectUtils.iterableEquality]);
    const message = pass ? () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + ((0, _jestMatcherUtils.stringify)(expected) === (0, _jestMatcherUtils.stringify)(received) ? '' : `Received:     ${(0, _jestMatcherUtils.printReceived)(received)}`) : () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (0, _jestMatcherUtils.printDiffOrStringify)(expected, received, EXPECTED_LABEL, RECEIVED_LABEL, isExpand(this.expand));

    // Passing the actual and expected objects so that a custom reporter
    // could access them, for example in order to display a custom visual diff,
    // or create a different error message
    return {
      actual: received,
      expected,
      message,
      name: matcherName,
      pass
    };
  },
  toHaveLength(received, expected) {
    const matcherName = 'toHaveLength';
    const isNot = this.isNot;
    const options = {
      isNot,
      promise: this.promise
    };
    if (typeof received?.length !== 'number') {
      throw new TypeError((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must have a length property whose value must be a number`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
    }
    (0, _jestMatcherUtils.ensureExpectedIsNonNegativeInteger)(expected, matcherName, options);
    const pass = received.length === expected;
    const message = () => {
      const labelExpected = 'Expected length';
      const labelReceivedLength = 'Received length';
      const labelReceivedValue = `Received ${(0, _getType.getType)(received)}`;
      const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(labelExpected, labelReceivedLength, labelReceivedValue);
      return (
        // eslint-disable-next-line prefer-template
        (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `${printLabel(labelExpected)}${isNot ? 'not ' : ''}${(0, _jestMatcherUtils.printExpected)(expected)}\n` + (isNot ? '' : `${printLabel(labelReceivedLength)}${(0, _jestMatcherUtils.printReceived)(received.length)}\n`) + `${printLabel(labelReceivedValue)}${isNot ? '    ' : ''}${(0, _jestMatcherUtils.printReceived)(received)}`
      );
    };
    return {
      message,
      pass
    };
  },
  toHaveProperty(received, expectedPath, expectedValue) {
    const matcherName = 'toHaveProperty';
    const expectedArgument = 'path';
    const hasValue = arguments.length === 3;
    const options = {
      isNot: this.isNot,
      promise: this.promise,
      secondArgument: hasValue ? 'value' : ''
    };
    if (received === null || received === undefined) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must not be null nor undefined`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
    }
    const expectedPathType = (0, _getType.getType)(expectedPath);
    if (expectedPathType !== 'string' && expectedPathType !== 'array') {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options), `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} path must be a string or array`, (0, _jestMatcherUtils.printWithType)('Expected', expectedPath, _jestMatcherUtils.printExpected)));
    }
    const expectedPathLength = typeof expectedPath === 'string' ? (0, _expectUtils.pathAsArray)(expectedPath).length : expectedPath.length;
    if (expectedPathType === 'array' && expectedPathLength === 0) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options), `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} path must not be an empty array`, (0, _jestMatcherUtils.printWithType)('Expected', expectedPath, _jestMatcherUtils.printExpected)));
    }
    const result = (0, _expectUtils.getPath)(received, expectedPath);
    const {
      lastTraversedObject,
      endPropIsDefined,
      hasEndProp,
      value
    } = result;
    const receivedPath = result.traversedPath;
    const hasCompletePath = receivedPath.length === expectedPathLength;
    const receivedValue = hasCompletePath ? result.value : lastTraversedObject;
    const pass = hasValue && endPropIsDefined ? (0, _expectUtils.equals)(value, expectedValue, [...this.customTesters, _expectUtils.iterableEquality]) : Boolean(hasEndProp);
    const message = pass ? () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options) + '\n\n' + (hasValue ? `Expected path: ${(0, _jestMatcherUtils.printExpected)(expectedPath)}\n\n` + `Expected value: not ${(0, _jestMatcherUtils.printExpected)(expectedValue)}${(0, _jestMatcherUtils.stringify)(expectedValue) === (0, _jestMatcherUtils.stringify)(receivedValue) ? '' : `\nReceived value:     ${(0, _jestMatcherUtils.printReceived)(receivedValue)}`}` : `Expected path: not ${(0, _jestMatcherUtils.printExpected)(expectedPath)}\n\n` + `Received value: ${(0, _jestMatcherUtils.printReceived)(receivedValue)}`) : () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options) + '\n\n' + `Expected path: ${(0, _jestMatcherUtils.printExpected)(expectedPath)}\n` + (hasCompletePath ? `\n${(0, _jestMatcherUtils.printDiffOrStringify)(expectedValue, receivedValue, EXPECTED_VALUE_LABEL, RECEIVED_VALUE_LABEL, isExpand(this.expand))}` : `Received path: ${(0, _jestMatcherUtils.printReceived)(expectedPathType === 'array' || receivedPath.length === 0 ? receivedPath : receivedPath.join('.'))}\n\n${hasValue ? `Expected value: ${(0, _jestMatcherUtils.printExpected)(expectedValue)}\n` : ''}Received value: ${(0, _jestMatcherUtils.printReceived)(receivedValue)}`);
    return {
      message,
      pass
    };
  },
  toMatch(received, expected) {
    const matcherName = 'toMatch';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    if (typeof received !== 'string') {
      throw new TypeError((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be a string`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
    }
    if (!(typeof expected === 'string') && !(expected && typeof expected.test === 'function')) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} value must be a string or regular expression`, (0, _jestMatcherUtils.printWithType)('Expected', expected, _jestMatcherUtils.printExpected)));
    }
    const pass = typeof expected === 'string' ? received.includes(expected) : new RegExp(expected).test(received);
    const message = pass ? () => typeof expected === 'string' ?
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected substring: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received string:        ${(0, _print.printReceivedStringContainExpectedSubstring)(received, received.indexOf(expected), expected.length)}` :
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected pattern: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received string:      ${(0, _print.printReceivedStringContainExpectedResult)(received, typeof expected.exec === 'function' ? expected.exec(received) : null)}` : () => {
      const labelExpected = `Expected ${typeof expected === 'string' ? 'substring' : 'pattern'}`;
      const labelReceived = 'Received string';
      const printLabel = (0, _jestMatcherUtils.getLabelPrinter)(labelExpected, labelReceived);
      return (
        // eslint-disable-next-line prefer-template
        (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `${printLabel(labelExpected)}${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `${printLabel(labelReceived)}${(0, _jestMatcherUtils.printReceived)(received)}`
      );
    };
    return {
      message,
      pass
    };
  },
  toMatchObject(received, expected) {
    const matcherName = 'toMatchObject';
    const options = {
      isNot: this.isNot,
      promise: this.promise
    };
    if (typeof received !== 'object' || received === null) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be a non-null object`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
    }
    if (typeof expected !== 'object' || expected === null) {
      throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} value must be a non-null object`, (0, _jestMatcherUtils.printWithType)('Expected', expected, _jestMatcherUtils.printExpected)));
    }
    const pass = (0, _expectUtils.equals)(received, expected, [...this.customTesters, _expectUtils.iterableEquality, _expectUtils.subsetEquality]);
    const message = pass ? () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}` + ((0, _jestMatcherUtils.stringify)(expected) === (0, _jestMatcherUtils.stringify)(received) ? '' : `\nReceived:     ${(0, _jestMatcherUtils.printReceived)(received)}`) : () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (0, _jestMatcherUtils.printDiffOrStringify)(expected, (0, _expectUtils.getObjectSubset)(received, expected, this.customTesters), EXPECTED_LABEL, RECEIVED_LABEL, isExpand(this.expand));
    return {
      message,
      pass
    };
  },
  toStrictEqual(received, expected) {
    const matcherName = 'toStrictEqual';
    const options = {
      comment: 'deep equality',
      isNot: this.isNot,
      promise: this.promise
    };
    const pass = (0, _expectUtils.equals)(received, expected, [...this.customTesters, ...toStrictEqualTesters], true);
    const message = pass ? () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + ((0, _jestMatcherUtils.stringify)(expected) === (0, _jestMatcherUtils.stringify)(received) ? '' : `Received:     ${(0, _jestMatcherUtils.printReceived)(received)}`) : () =>
    // eslint-disable-next-line prefer-template
    (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (0, _jestMatcherUtils.printDiffOrStringify)(expected, received, EXPECTED_LABEL, RECEIVED_LABEL, isExpand(this.expand));

    // Passing the actual and expected objects so that a custom reporter
    // could access them, for example in order to display a custom visual diff,
    // or create a different error message
    return {
      actual: received,
      expected,
      message,
      name: matcherName,
      pass
    };
  }
};
var _default = exports["default"] = matchers;

/***/ },

/***/ "./src/print.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.printReceivedStringContainExpectedSubstring = exports.printReceivedStringContainExpectedResult = exports.printReceivedConstructorNameNot = exports.printReceivedConstructorName = exports.printReceivedArrayContainExpectedItem = exports.printExpectedConstructorNameNot = exports.printExpectedConstructorName = exports.printCloseTo = void 0;
var _jestMatcherUtils = require("jest-matcher-utils");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

// Format substring but do not enclose in double quote marks.
// The replacement is compatible with pretty-format package.
const printSubstring = val => val.replaceAll(/"|\\/g, '\\$&');
const printReceivedStringContainExpectedSubstring = (received, start, length // not end
) => (0, _jestMatcherUtils.RECEIVED_COLOR)(`"${printSubstring(received.slice(0, start))}${(0, _jestMatcherUtils.INVERTED_COLOR)(printSubstring(received.slice(start, start + length)))}${printSubstring(received.slice(start + length))}"`);
exports.printReceivedStringContainExpectedSubstring = printReceivedStringContainExpectedSubstring;
const printReceivedStringContainExpectedResult = (received, result) => result === null ? (0, _jestMatcherUtils.printReceived)(received) : printReceivedStringContainExpectedSubstring(received, result.index, result[0].length);

// The serialized array is compatible with pretty-format package min option.
// However, items have default stringify depth (instead of depth - 1)
// so expected item looks consistent by itself and enclosed in the array.
exports.printReceivedStringContainExpectedResult = printReceivedStringContainExpectedResult;
const printReceivedArrayContainExpectedItem = (received, index) => (0, _jestMatcherUtils.RECEIVED_COLOR)(`[${received.map((item, i) => {
  const stringified = (0, _jestMatcherUtils.stringify)(item);
  return i === index ? (0, _jestMatcherUtils.INVERTED_COLOR)(stringified) : stringified;
}).join(', ')}]`);
exports.printReceivedArrayContainExpectedItem = printReceivedArrayContainExpectedItem;
const printCloseTo = (receivedDiff, expectedDiff, precision, isNot) => {
  const receivedDiffString = (0, _jestMatcherUtils.stringify)(receivedDiff);
  const expectedDiffString = receivedDiffString.includes('e') ?
  // toExponential arg is number of digits after the decimal point.
  expectedDiff.toExponential(0) : 0 <= precision && precision < 20 ?
  // toFixed arg is number of digits after the decimal point.
  // It may be a value between 0 and 20 inclusive.
  // Implementations may optionally support a larger range of values.
  expectedDiff.toFixed(precision + 1) : (0, _jestMatcherUtils.stringify)(expectedDiff);
  return `Expected precision:  ${isNot ? '    ' : ''}  ${(0, _jestMatcherUtils.stringify)(precision)}\n` + `Expected difference: ${isNot ? 'not ' : ''}< ${(0, _jestMatcherUtils.EXPECTED_COLOR)(expectedDiffString)}\n` + `Received difference: ${isNot ? '    ' : ''}  ${(0, _jestMatcherUtils.RECEIVED_COLOR)(receivedDiffString)}`;
};
exports.printCloseTo = printCloseTo;
const printExpectedConstructorName = (label, expected) => `${printConstructorName(label, expected, false, true)}\n`;
exports.printExpectedConstructorName = printExpectedConstructorName;
const printExpectedConstructorNameNot = (label, expected) => `${printConstructorName(label, expected, true, true)}\n`;
exports.printExpectedConstructorNameNot = printExpectedConstructorNameNot;
const printReceivedConstructorName = (label, received) => `${printConstructorName(label, received, false, false)}\n`;

// Do not call function if received is equal to expected.
exports.printReceivedConstructorName = printReceivedConstructorName;
const printReceivedConstructorNameNot = (label, received, expected) => typeof expected.name === 'string' && expected.name.length > 0 && typeof received.name === 'string' && received.name.length > 0 ? `${printConstructorName(label, received, true, false)} ${Object.getPrototypeOf(received) === expected ? 'extends' : 'extends … extends'} ${(0, _jestMatcherUtils.EXPECTED_COLOR)(expected.name)}\n` : `${printConstructorName(label, received, false, false)}\n`;
exports.printReceivedConstructorNameNot = printReceivedConstructorNameNot;
const printConstructorName = (label, constructor, isNot, isExpected) => typeof constructor.name === 'string' ? constructor.name.length === 0 ? `${label} name is an empty string` : `${label}: ${isNot ? isExpected ? 'not ' : '    ' : ''}${isExpected ? (0, _jestMatcherUtils.EXPECTED_COLOR)(constructor.name) : (0, _jestMatcherUtils.RECEIVED_COLOR)(constructor.name)}` : `${label} name is not a string`;

/***/ },

/***/ "./src/spyMatchers.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _expectUtils = require("@jest/expect-utils");
var _getType = require("@jest/get-type");
var _jestMatcherUtils = require("jest-matcher-utils");
var _jestMatchersObject = __webpack_require__("./src/jestMatchersObject.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/* eslint-disable unicorn/consistent-function-scoping */

// The optional property of matcher context is true if undefined.
const isExpand = expand => expand !== false;
const PRINT_LIMIT = 3;
const NO_ARGUMENTS = 'called with 0 arguments';
const printExpectedArgs = expected => expected.length === 0 ? NO_ARGUMENTS : expected.map(arg => (0, _jestMatcherUtils.printExpected)(arg)).join(', ');
const printReceivedArgs = (received, expected) => received.length === 0 ? NO_ARGUMENTS : received.map((arg, i) => Array.isArray(expected) && i < expected.length && isEqualValue(expected[i], arg) ? printCommon(arg) : (0, _jestMatcherUtils.printReceived)(arg)).join(', ');
const printCommon = val => (0, _jestMatcherUtils.DIM_COLOR)((0, _jestMatcherUtils.stringify)(val));
const isEqualValue = (expected, received) => (0, _expectUtils.equals)(expected, received, [...(0, _jestMatchersObject.getCustomEqualityTesters)(), _expectUtils.iterableEquality]);
const isEqualCall = (expected, received) => received.length === expected.length && isEqualValue(expected, received);
const isEqualReturn = (expected, result) => result.type === 'return' && isEqualValue(expected, result.value);
const countReturns = results => results.reduce((n, result) => result.type === 'return' ? n + 1 : n, 0);
const printNumberOfReturns = (countReturns, countCalls) => `\nNumber of returns: ${(0, _jestMatcherUtils.printReceived)(countReturns)}${countCalls === countReturns ? '' : `\nNumber of calls:   ${(0, _jestMatcherUtils.printReceived)(countCalls)}`}`;
// Given a label, return a function which given a string,
// right-aligns it preceding the colon in the label.
const getRightAlignedPrinter = label => {
  // Assume that the label contains a colon.
  const index = label.indexOf(':');
  const suffix = label.slice(index);
  return (string, isExpectedCall) => (isExpectedCall ? `->${' '.repeat(Math.max(0, index - 2 - string.length))}` : ' '.repeat(Math.max(index - string.length))) + string + suffix;
};
const printReceivedCallsNegative = (expected, indexedCalls, isOnlyCall, iExpectedCall) => {
  if (indexedCalls.length === 0) {
    return '';
  }
  const label = 'Received:     ';
  if (isOnlyCall) {
    return `${label + printReceivedArgs(indexedCalls[0], expected)}\n`;
  }
  const printAligned = getRightAlignedPrinter(label);
  return `Received\n${indexedCalls.reduce((printed, [i, args]) => `${printed + printAligned(String(i + 1), i === iExpectedCall) + printReceivedArgs(args, expected)}\n`, '')}`;
};
const printExpectedReceivedCallsPositive = (expected, indexedCalls, expand, isOnlyCall, iExpectedCall) => {
  const expectedLine = `Expected: ${printExpectedArgs(expected)}\n`;
  if (indexedCalls.length === 0) {
    return expectedLine;
  }
  const label = 'Received: ';
  if (isOnlyCall && (iExpectedCall === 0 || iExpectedCall === undefined)) {
    const received = indexedCalls[0][1];
    if (isLineDiffableCall(expected, received)) {
      // Display diff without indentation.
      const lines = [(0, _jestMatcherUtils.EXPECTED_COLOR)('- Expected'), (0, _jestMatcherUtils.RECEIVED_COLOR)('+ Received'), ''];
      const length = Math.max(expected.length, received.length);
      for (let i = 0; i < length; i += 1) {
        if (i < expected.length && i < received.length) {
          if (isEqualValue(expected[i], received[i])) {
            lines.push(`  ${printCommon(received[i])},`);
            continue;
          }
          if (isLineDiffableArg(expected[i], received[i])) {
            const difference = (0, _jestMatcherUtils.diff)(expected[i], received[i], {
              expand
            });
            if (typeof difference === 'string' && difference.includes('- Expected') && difference.includes('+ Received')) {
              // Omit annotation in case multiple args have diff.
              lines.push(`${difference.split('\n').slice(3).join('\n')},`);
              continue;
            }
          }
        }
        if (i < expected.length) {
          lines.push(`${(0, _jestMatcherUtils.EXPECTED_COLOR)(`- ${(0, _jestMatcherUtils.stringify)(expected[i])}`)},`);
        }
        if (i < received.length) {
          lines.push(`${(0, _jestMatcherUtils.RECEIVED_COLOR)(`+ ${(0, _jestMatcherUtils.stringify)(received[i])}`)},`);
        }
      }
      return `${lines.join('\n')}\n`;
    }
    return `${expectedLine + label + printReceivedArgs(received, expected)}\n`;
  }
  const printAligned = getRightAlignedPrinter(label);
  return (
    // eslint-disable-next-line prefer-template
    expectedLine + 'Received\n' + indexedCalls.reduce((printed, [i, received]) => {
      const aligned = printAligned(String(i + 1), i === iExpectedCall);
      return `${printed + ((i === iExpectedCall || iExpectedCall === undefined) && isLineDiffableCall(expected, received) ? aligned.replace(': ', '\n') + printDiffCall(expected, received, expand) : aligned + printReceivedArgs(received, expected))}\n`;
    }, '')
  );
};
const indentation = 'Received'.replaceAll(/\w/g, ' ');
const printDiffCall = (expected, received, expand) => received.map((arg, i) => {
  if (i < expected.length) {
    if (isEqualValue(expected[i], arg)) {
      return `${indentation}  ${printCommon(arg)},`;
    }
    if (isLineDiffableArg(expected[i], arg)) {
      const difference = (0, _jestMatcherUtils.diff)(expected[i], arg, {
        expand
      });
      if (typeof difference === 'string' && difference.includes('- Expected') && difference.includes('+ Received')) {
        // Display diff with indentation.
        // Omit annotation in case multiple args have diff.
        return `${difference.split('\n').slice(3).map(line => indentation + line).join('\n')},`;
      }
    }
  }

  // Display + only if received arg has no corresponding expected arg.
  return `${indentation + (i < expected.length ? `  ${(0, _jestMatcherUtils.printReceived)(arg)}` : (0, _jestMatcherUtils.RECEIVED_COLOR)(`+ ${(0, _jestMatcherUtils.stringify)(arg)}`))},`;
}).join('\n');
const isLineDiffableCall = (expected, received) => expected.some((arg, i) => i < received.length && isLineDiffableArg(arg, received[i]));

// Almost redundant with function in jest-matcher-utils,
// except no line diff for any strings.
const isLineDiffableArg = (expected, received) => {
  const expectedType = (0, _getType.getType)(expected);
  const receivedType = (0, _getType.getType)(received);
  if (expectedType !== receivedType) {
    return false;
  }
  if ((0, _getType.isPrimitive)(expected)) {
    return false;
  }
  if (expectedType === 'date' || expectedType === 'function' || expectedType === 'regexp') {
    return false;
  }
  if (expected instanceof Error && received instanceof Error) {
    return false;
  }
  if (expectedType === 'object' && typeof expected.asymmetricMatch === 'function') {
    return false;
  }
  if (receivedType === 'object' && typeof received.asymmetricMatch === 'function') {
    return false;
  }
  return true;
};
const printResult = (result, expected) => result.type === 'throw' ? 'function call threw an error' : result.type === 'incomplete' ? 'function call has not returned yet' : isEqualValue(expected, result.value) ? printCommon(result.value) : (0, _jestMatcherUtils.printReceived)(result.value);
// Return either empty string or one line per indexed result,
// so additional empty line can separate from `Number of returns` which follows.
const printReceivedResults = (label, expected, indexedResults, isOnlyCall, iExpectedCall) => {
  if (indexedResults.length === 0) {
    return '';
  }
  if (isOnlyCall && (iExpectedCall === 0 || iExpectedCall === undefined)) {
    return `${label + printResult(indexedResults[0][1], expected)}\n`;
  }
  const printAligned = getRightAlignedPrinter(label);
  return (
    // eslint-disable-next-line prefer-template
    label.replace(':', '').trim() + '\n' + indexedResults.reduce((printed, [i, result]) => `${printed + printAligned(String(i + 1), i === iExpectedCall) + printResult(result, expected)}\n`, '')
  );
};
const createToHaveBeenCalledMatcher = () => function (received, expected) {
  const expectedArgument = '';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  (0, _jestMatcherUtils.ensureNoExpected)(expected, 'toHaveBeenCalled', options);
  ensureMockOrSpy(received, 'toHaveBeenCalled', expectedArgument, options);
  const receivedIsSpy = isSpy(received);
  const receivedName = receivedIsSpy ? 'spy' : received.getMockName();
  const count = receivedIsSpy ? received.calls.count() : received.mock.calls.length;
  const calls = receivedIsSpy ? received.calls.all().map(x => x.args) : received.mock.calls;
  const pass = count > 0;
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveBeenCalled', receivedName, expectedArgument, options) + '\n\n' + `Expected number of calls: ${(0, _jestMatcherUtils.printExpected)(0)}\n` + `Received number of calls: ${(0, _jestMatcherUtils.printReceived)(count)}\n\n` + calls.reduce((lines, args, i) => {
    if (lines.length < PRINT_LIMIT) {
      lines.push(`${i + 1}: ${printReceivedArgs(args)}`);
    }
    return lines;
  }, []).join('\n') : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveBeenCalled', receivedName, expectedArgument, options) + '\n\n' + `Expected number of calls: >= ${(0, _jestMatcherUtils.printExpected)(1)}\n` + `Received number of calls:    ${(0, _jestMatcherUtils.printReceived)(count)}`;
  return {
    message,
    pass
  };
};
const createToHaveReturnedMatcher = () => function (received, expected) {
  const expectedArgument = '';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  (0, _jestMatcherUtils.ensureNoExpected)(expected, 'toHaveReturned', options);
  ensureMock(received, 'toHaveReturned', expectedArgument, options);
  const receivedName = received.getMockName();

  // Count return values that correspond only to calls that returned
  const count = received.mock.results.reduce((n, result) => result.type === 'return' ? n + 1 : n, 0);
  const pass = count > 0;
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveReturned', receivedName, expectedArgument, options) + '\n\n' + `Expected number of returns: ${(0, _jestMatcherUtils.printExpected)(0)}\n` + `Received number of returns: ${(0, _jestMatcherUtils.printReceived)(count)}\n\n` + received.mock.results.reduce((lines, result, i) => {
    if (result.type === 'return' && lines.length < PRINT_LIMIT) {
      lines.push(`${i + 1}: ${(0, _jestMatcherUtils.printReceived)(result.value)}`);
    }
    return lines;
  }, []).join('\n') + (received.mock.calls.length === count ? '' : `\n\nReceived number of calls:   ${(0, _jestMatcherUtils.printReceived)(received.mock.calls.length)}`) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveReturned', receivedName, expectedArgument, options) + '\n\n' + `Expected number of returns: >= ${(0, _jestMatcherUtils.printExpected)(1)}\n` + `Received number of returns:    ${(0, _jestMatcherUtils.printReceived)(count)}` + (received.mock.calls.length === count ? '' : `\nReceived number of calls:      ${(0, _jestMatcherUtils.printReceived)(received.mock.calls.length)}`);
  return {
    message,
    pass
  };
};
const createToHaveBeenCalledTimesMatcher = () => function (received, expected) {
  const expectedArgument = 'expected';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  (0, _jestMatcherUtils.ensureExpectedIsNonNegativeInteger)(expected, 'toHaveBeenCalledTimes', options);
  ensureMockOrSpy(received, 'toHaveBeenCalledTimes', expectedArgument, options);
  const receivedIsSpy = isSpy(received);
  const receivedName = receivedIsSpy ? 'spy' : received.getMockName();
  const count = receivedIsSpy ? received.calls.count() : received.mock.calls.length;
  const pass = count === expected;
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveBeenCalledTimes', receivedName, expectedArgument, options) + '\n\n' + `Expected number of calls: not ${(0, _jestMatcherUtils.printExpected)(expected)}` : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveBeenCalledTimes', receivedName, expectedArgument, options) + '\n\n' + `Expected number of calls: ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received number of calls: ${(0, _jestMatcherUtils.printReceived)(count)}`;
  return {
    message,
    pass
  };
};
const createToHaveReturnedTimesMatcher = () => function (received, expected) {
  const expectedArgument = 'expected';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  (0, _jestMatcherUtils.ensureExpectedIsNonNegativeInteger)(expected, 'toHaveReturnedTimes', options);
  ensureMock(received, 'toHaveReturnedTimes', expectedArgument, options);
  const receivedName = received.getMockName();

  // Count return values that correspond only to calls that returned
  const count = received.mock.results.reduce((n, result) => result.type === 'return' ? n + 1 : n, 0);
  const pass = count === expected;
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveReturnedTimes', receivedName, expectedArgument, options) + '\n\n' + `Expected number of returns: not ${(0, _jestMatcherUtils.printExpected)(expected)}` + (received.mock.calls.length === count ? '' : `\n\nReceived number of calls:       ${(0, _jestMatcherUtils.printReceived)(received.mock.calls.length)}`) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)('toHaveReturnedTimes', receivedName, expectedArgument, options) + '\n\n' + `Expected number of returns: ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + `Received number of returns: ${(0, _jestMatcherUtils.printReceived)(count)}` + (received.mock.calls.length === count ? '' : `\nReceived number of calls:   ${(0, _jestMatcherUtils.printReceived)(received.mock.calls.length)}`);
  return {
    message,
    pass
  };
};
const createToHaveBeenCalledWithMatcher = () => function (received, ...expected) {
  const expectedArgument = '...expected';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  ensureMockOrSpy(received, 'toHaveBeenCalledWith', expectedArgument, options);
  const receivedIsSpy = isSpy(received);
  const receivedName = receivedIsSpy ? 'spy' : received.getMockName();
  const calls = receivedIsSpy ? received.calls.all().map(x => x.args) : received.mock.calls;
  const pass = calls.some(call => isEqualCall(expected, call));
  const message = pass ? () => {
    // Some examples of calls that are equal to expected value.
    const indexedCalls = [];
    let i = 0;
    while (i < calls.length && indexedCalls.length < PRINT_LIMIT) {
      if (isEqualCall(expected, calls[i])) {
        indexedCalls.push([i, calls[i]]);
      }
      i += 1;
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveBeenCalledWith', receivedName, expectedArgument, options) + '\n\n' + `Expected: not ${printExpectedArgs(expected)}\n` + (calls.length === 1 && (0, _jestMatcherUtils.stringify)(calls[0]) === (0, _jestMatcherUtils.stringify)(expected) ? '' : printReceivedCallsNegative(expected, indexedCalls, calls.length === 1)) + `\nNumber of calls: ${(0, _jestMatcherUtils.printReceived)(calls.length)}`
    );
  } : () => {
    // Some examples of calls that are not equal to expected value.
    const indexedCalls = [];
    let i = 0;
    while (i < calls.length && indexedCalls.length < PRINT_LIMIT) {
      indexedCalls.push([i, calls[i]]);
      i += 1;
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveBeenCalledWith', receivedName, expectedArgument, options) + '\n\n' + printExpectedReceivedCallsPositive(expected, indexedCalls, isExpand(this.expand), calls.length === 1) + `\nNumber of calls: ${(0, _jestMatcherUtils.printReceived)(calls.length)}`
    );
  };
  return {
    message,
    pass
  };
};
const createToHaveReturnedWithMatcher = () => function (received, expected) {
  const expectedArgument = 'expected';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  ensureMock(received, 'toHaveReturnedWith', expectedArgument, options);
  const receivedName = received.getMockName();
  const {
    calls,
    results
  } = received.mock;
  const pass = results.some(result => isEqualReturn(expected, result));
  const message = pass ? () => {
    // Some examples of results that are equal to expected value.
    const indexedResults = [];
    let i = 0;
    while (i < results.length && indexedResults.length < PRINT_LIMIT) {
      if (isEqualReturn(expected, results[i])) {
        indexedResults.push([i, results[i]]);
      }
      i += 1;
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveReturnedWith', receivedName, expectedArgument, options) + '\n\n' + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + (results.length === 1 && results[0].type === 'return' && (0, _jestMatcherUtils.stringify)(results[0].value) === (0, _jestMatcherUtils.stringify)(expected) ? '' : printReceivedResults('Received:     ', expected, indexedResults, results.length === 1)) + printNumberOfReturns(countReturns(results), calls.length)
    );
  } : () => {
    // Some examples of results that are not equal to expected value.
    const indexedResults = [];
    let i = 0;
    while (i < results.length && indexedResults.length < PRINT_LIMIT) {
      indexedResults.push([i, results[i]]);
      i += 1;
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveReturnedWith', receivedName, expectedArgument, options) + '\n\n' + `Expected: ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + printReceivedResults('Received: ', expected, indexedResults, results.length === 1) + printNumberOfReturns(countReturns(results), calls.length)
    );
  };
  return {
    message,
    pass
  };
};
const createToHaveBeenLastCalledWithMatcher = () => function (received, ...expected) {
  const expectedArgument = '...expected';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  ensureMockOrSpy(received, 'toHaveBeenLastCalledWith', expectedArgument, options);
  const receivedIsSpy = isSpy(received);
  const receivedName = receivedIsSpy ? 'spy' : received.getMockName();
  const calls = receivedIsSpy ? received.calls.all().map(x => x.args) : received.mock.calls;
  const iLast = calls.length - 1;
  const pass = iLast >= 0 && isEqualCall(expected, calls[iLast]);
  const message = pass ? () => {
    const indexedCalls = [];
    if (iLast > 0) {
      // Display preceding call as context.
      indexedCalls.push([iLast - 1, calls[iLast - 1]]);
    }
    indexedCalls.push([iLast, calls[iLast]]);
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveBeenLastCalledWith', receivedName, expectedArgument, options) + '\n\n' + `Expected: not ${printExpectedArgs(expected)}\n` + (calls.length === 1 && (0, _jestMatcherUtils.stringify)(calls[0]) === (0, _jestMatcherUtils.stringify)(expected) ? '' : printReceivedCallsNegative(expected, indexedCalls, calls.length === 1, iLast)) + `\nNumber of calls: ${(0, _jestMatcherUtils.printReceived)(calls.length)}`
    );
  } : () => {
    const indexedCalls = [];
    if (iLast >= 0) {
      if (iLast > 0) {
        let i = iLast - 1;
        // Is there a preceding call that is equal to expected args?
        while (i >= 0 && !isEqualCall(expected, calls[i])) {
          i -= 1;
        }
        if (i < 0) {
          i = iLast - 1; // otherwise, preceding call
        }
        indexedCalls.push([i, calls[i]]);
      }
      indexedCalls.push([iLast, calls[iLast]]);
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveBeenLastCalledWith', receivedName, expectedArgument, options) + '\n\n' + printExpectedReceivedCallsPositive(expected, indexedCalls, isExpand(this.expand), calls.length === 1, iLast) + `\nNumber of calls: ${(0, _jestMatcherUtils.printReceived)(calls.length)}`
    );
  };
  return {
    message,
    pass
  };
};
const createToHaveLastReturnedWithMatcher = () => function (received, expected) {
  const expectedArgument = 'expected';
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  ensureMock(received, 'toHaveLastReturnedWith', expectedArgument, options);
  const receivedName = received.getMockName();
  const {
    calls,
    results
  } = received.mock;
  const iLast = results.length - 1;
  const pass = iLast >= 0 && isEqualReturn(expected, results[iLast]);
  const message = pass ? () => {
    const indexedResults = [];
    if (iLast > 0) {
      // Display preceding result as context.
      indexedResults.push([iLast - 1, results[iLast - 1]]);
    }
    indexedResults.push([iLast, results[iLast]]);
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveLastReturnedWith', receivedName, expectedArgument, options) + '\n\n' + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + (results.length === 1 && results[0].type === 'return' && (0, _jestMatcherUtils.stringify)(results[0].value) === (0, _jestMatcherUtils.stringify)(expected) ? '' : printReceivedResults('Received:     ', expected, indexedResults, results.length === 1, iLast)) + printNumberOfReturns(countReturns(results), calls.length)
    );
  } : () => {
    const indexedResults = [];
    if (iLast >= 0) {
      if (iLast > 0) {
        let i = iLast - 1;
        // Is there a preceding result that is equal to expected value?
        while (i >= 0 && !isEqualReturn(expected, results[i])) {
          i -= 1;
        }
        if (i < 0) {
          i = iLast - 1; // otherwise, preceding result
        }
        indexedResults.push([i, results[i]]);
      }
      indexedResults.push([iLast, results[iLast]]);
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveLastReturnedWith', receivedName, expectedArgument, options) + '\n\n' + `Expected: ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + printReceivedResults('Received: ', expected, indexedResults, results.length === 1, iLast) + printNumberOfReturns(countReturns(results), calls.length)
    );
  };
  return {
    message,
    pass
  };
};
const createToHaveBeenNthCalledWithMatcher = () => function (received, nth, ...expected) {
  const expectedArgument = 'n';
  const options = {
    expectedColor: arg => arg,
    isNot: this.isNot,
    promise: this.promise,
    secondArgument: '...expected'
  };
  ensureMockOrSpy(received, 'toHaveBeenNthCalledWith', expectedArgument, options);
  if (!Number.isSafeInteger(nth) || nth < 1) {
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)('toHaveBeenNthCalledWith', undefined, expectedArgument, options), `${expectedArgument} must be a positive integer`, (0, _jestMatcherUtils.printWithType)(expectedArgument, nth, _jestMatcherUtils.stringify)));
  }
  const receivedIsSpy = isSpy(received);
  const receivedName = receivedIsSpy ? 'spy' : received.getMockName();
  const calls = receivedIsSpy ? received.calls.all().map(x => x.args) : received.mock.calls;
  const length = calls.length;
  const iNth = nth - 1;
  const pass = iNth < length && isEqualCall(expected, calls[iNth]);
  const message = pass ? () => {
    // Display preceding and following calls,
    // in case assertions fails because index is off by one.
    const indexedCalls = [];
    if (iNth - 1 >= 0) {
      indexedCalls.push([iNth - 1, calls[iNth - 1]]);
    }
    indexedCalls.push([iNth, calls[iNth]]);
    if (iNth + 1 < length) {
      indexedCalls.push([iNth + 1, calls[iNth + 1]]);
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveBeenNthCalledWith', receivedName, expectedArgument, options) + '\n\n' + `n: ${nth}\n` + `Expected: not ${printExpectedArgs(expected)}\n` + (calls.length === 1 && (0, _jestMatcherUtils.stringify)(calls[0]) === (0, _jestMatcherUtils.stringify)(expected) ? '' : printReceivedCallsNegative(expected, indexedCalls, calls.length === 1, iNth)) + `\nNumber of calls: ${(0, _jestMatcherUtils.printReceived)(calls.length)}`
    );
  } : () => {
    // Display preceding and following calls:
    // * nearest call that is equal to expected args
    // * otherwise, adjacent call
    // in case assertions fails because of index, especially off by one.
    const indexedCalls = [];
    if (iNth < length) {
      if (iNth - 1 >= 0) {
        let i = iNth - 1;
        // Is there a preceding call that is equal to expected args?
        while (i >= 0 && !isEqualCall(expected, calls[i])) {
          i -= 1;
        }
        if (i < 0) {
          i = iNth - 1; // otherwise, adjacent call
        }
        indexedCalls.push([i, calls[i]]);
      }
      indexedCalls.push([iNth, calls[iNth]]);
      if (iNth + 1 < length) {
        let i = iNth + 1;
        // Is there a following call that is equal to expected args?
        while (i < length && !isEqualCall(expected, calls[i])) {
          i += 1;
        }
        if (i >= length) {
          i = iNth + 1; // otherwise, adjacent call
        }
        indexedCalls.push([i, calls[i]]);
      }
    } else if (length > 0) {
      // The number of received calls is fewer than the expected number.
      let i = length - 1;
      // Is there a call that is equal to expected args?
      while (i >= 0 && !isEqualCall(expected, calls[i])) {
        i -= 1;
      }
      if (i < 0) {
        i = length - 1; // otherwise, last call
      }
      indexedCalls.push([i, calls[i]]);
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveBeenNthCalledWith', receivedName, expectedArgument, options) + '\n\n' + `n: ${nth}\n` + printExpectedReceivedCallsPositive(expected, indexedCalls, isExpand(this.expand), calls.length === 1, iNth) + `\nNumber of calls: ${(0, _jestMatcherUtils.printReceived)(calls.length)}`
    );
  };
  return {
    message,
    pass
  };
};
const createToHaveNthReturnedWithMatcher = () => function (received, nth, expected) {
  const expectedArgument = 'n';
  const options = {
    expectedColor: arg => arg,
    isNot: this.isNot,
    promise: this.promise,
    secondArgument: 'expected'
  };
  ensureMock(received, 'toHaveNthReturnedWith', expectedArgument, options);
  if (!Number.isSafeInteger(nth) || nth < 1) {
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)('toHaveNthReturnedWith', undefined, expectedArgument, options), `${expectedArgument} must be a positive integer`, (0, _jestMatcherUtils.printWithType)(expectedArgument, nth, _jestMatcherUtils.stringify)));
  }
  const receivedName = received.getMockName();
  const {
    calls,
    results
  } = received.mock;
  const length = results.length;
  const iNth = nth - 1;
  const pass = iNth < length && isEqualReturn(expected, results[iNth]);
  const message = pass ? () => {
    // Display preceding and following results,
    // in case assertions fails because index is off by one.
    const indexedResults = [];
    if (iNth - 1 >= 0) {
      indexedResults.push([iNth - 1, results[iNth - 1]]);
    }
    indexedResults.push([iNth, results[iNth]]);
    if (iNth + 1 < length) {
      indexedResults.push([iNth + 1, results[iNth + 1]]);
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveNthReturnedWith', receivedName, expectedArgument, options) + '\n\n' + `n: ${nth}\n` + `Expected: not ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + (results.length === 1 && results[0].type === 'return' && (0, _jestMatcherUtils.stringify)(results[0].value) === (0, _jestMatcherUtils.stringify)(expected) ? '' : printReceivedResults('Received:     ', expected, indexedResults, results.length === 1, iNth)) + printNumberOfReturns(countReturns(results), calls.length)
    );
  } : () => {
    // Display preceding and following results:
    // * nearest result that is equal to expected value
    // * otherwise, adjacent result
    // in case assertions fails because of index, especially off by one.
    const indexedResults = [];
    if (iNth < length) {
      if (iNth - 1 >= 0) {
        let i = iNth - 1;
        // Is there a preceding result that is equal to expected value?
        while (i >= 0 && !isEqualReturn(expected, results[i])) {
          i -= 1;
        }
        if (i < 0) {
          i = iNth - 1; // otherwise, adjacent result
        }
        indexedResults.push([i, results[i]]);
      }
      indexedResults.push([iNth, results[iNth]]);
      if (iNth + 1 < length) {
        let i = iNth + 1;
        // Is there a following result that is equal to expected value?
        while (i < length && !isEqualReturn(expected, results[i])) {
          i += 1;
        }
        if (i >= length) {
          i = iNth + 1; // otherwise, adjacent result
        }
        indexedResults.push([i, results[i]]);
      }
    } else if (length > 0) {
      // The number of received calls is fewer than the expected number.
      let i = length - 1;
      // Is there a result that is equal to expected value?
      while (i >= 0 && !isEqualReturn(expected, results[i])) {
        i -= 1;
      }
      if (i < 0) {
        i = length - 1; // otherwise, last result
      }
      indexedResults.push([i, results[i]]);
    }
    return (
      // eslint-disable-next-line prefer-template
      (0, _jestMatcherUtils.matcherHint)('toHaveNthReturnedWith', receivedName, expectedArgument, options) + '\n\n' + `n: ${nth}\n` + `Expected: ${(0, _jestMatcherUtils.printExpected)(expected)}\n` + printReceivedResults('Received: ', expected, indexedResults, results.length === 1, iNth) + printNumberOfReturns(countReturns(results), calls.length)
    );
  };
  return {
    message,
    pass
  };
};
const spyMatchers = {
  toHaveBeenCalled: createToHaveBeenCalledMatcher(),
  toHaveBeenCalledTimes: createToHaveBeenCalledTimesMatcher(),
  toHaveBeenCalledWith: createToHaveBeenCalledWithMatcher(),
  toHaveBeenLastCalledWith: createToHaveBeenLastCalledWithMatcher(),
  toHaveBeenNthCalledWith: createToHaveBeenNthCalledWithMatcher(),
  toHaveLastReturnedWith: createToHaveLastReturnedWithMatcher(),
  toHaveNthReturnedWith: createToHaveNthReturnedWithMatcher(),
  toHaveReturned: createToHaveReturnedMatcher(),
  toHaveReturnedTimes: createToHaveReturnedTimesMatcher(),
  toHaveReturnedWith: createToHaveReturnedWithMatcher()
};
const isMock = received => received != null && received._isMockFunction === true;
const isSpy = received => received != null && received.calls != null && typeof received.calls.all === 'function' && typeof received.calls.count === 'function';
const ensureMockOrSpy = (received, matcherName, expectedArgument, options) => {
  if (!isMock(received) && !isSpy(received)) {
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be a mock or spy function`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
  }
};
const ensureMock = (received, matcherName, expectedArgument, options) => {
  if (!isMock(received)) {
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, expectedArgument, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be a mock function`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
  }
};
var _default = exports["default"] = spyMatchers;

/***/ },

/***/ "./src/toThrowMatchers.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = exports.createMatcher = void 0;
var _expectUtils = require("@jest/expect-utils");
var _jestMatcherUtils = require("jest-matcher-utils");
var _jestMessageUtil = require("jest-message-util");
var _print = __webpack_require__("./src/print.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const DID_NOT_THROW = 'Received function did not throw';
const getThrown = e => {
  const hasMessage = e !== null && e !== undefined && typeof e.message === 'string';
  if (hasMessage && typeof e.name === 'string' && typeof e.stack === 'string') {
    return {
      hasMessage,
      isError: true,
      message: e.message,
      value: e
    };
  }
  return {
    hasMessage,
    isError: false,
    message: hasMessage ? e.message : String(e),
    value: e
  };
};
const createMatcher = (matcherName, fromPromise) => function (received, expected) {
  const options = {
    isNot: this.isNot,
    promise: this.promise
  };
  let thrown = null;
  if (fromPromise && (0, _expectUtils.isError)(received)) {
    thrown = getThrown(received);
  } else {
    if (typeof received === 'function') {
      try {
        received();
      } catch (error) {
        thrown = getThrown(error);
      }
    } else {
      if (!fromPromise) {
        const placeholder = expected === undefined ? '' : 'expected';
        throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, placeholder, options), `${(0, _jestMatcherUtils.RECEIVED_COLOR)('received')} value must be a function`, (0, _jestMatcherUtils.printWithType)('Received', received, _jestMatcherUtils.printReceived)));
      }
    }
  }
  if (expected === undefined) {
    return toThrow(matcherName, options, thrown);
  } else if (typeof expected === 'function') {
    return toThrowExpectedClass(matcherName, options, thrown, expected);
  } else if (typeof expected === 'string') {
    return toThrowExpectedString(matcherName, options, thrown, expected);
  } else if (expected !== null && typeof expected.test === 'function') {
    return toThrowExpectedRegExp(matcherName, options, thrown, expected);
  } else if (expected !== null && typeof expected.asymmetricMatch === 'function') {
    return toThrowExpectedAsymmetric(matcherName, options, thrown, expected);
  } else if (expected !== null && typeof expected === 'object') {
    return toThrowExpectedObject(matcherName, options, thrown, expected);
  } else {
    throw new Error((0, _jestMatcherUtils.matcherErrorMessage)((0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options), `${(0, _jestMatcherUtils.EXPECTED_COLOR)('expected')} value must be a string or regular expression or class or error`, (0, _jestMatcherUtils.printWithType)('Expected', expected, _jestMatcherUtils.printExpected)));
  }
};
exports.createMatcher = createMatcher;
const matchers = {
  toThrow: createMatcher('toThrow')
};
const toThrowExpectedRegExp = (matcherName, options, thrown, expected) => {
  const pass = thrown !== null && expected.test(thrown.message);
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + formatExpected('Expected pattern: not ', expected) + (thrown !== null && thrown.hasMessage ? formatReceived('Received message:     ', thrown, 'message', expected) + formatStack(thrown) : formatReceived('Received value:       ', thrown, 'value')) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + formatExpected('Expected pattern: ', expected) + (thrown === null ? `\n${DID_NOT_THROW}` : thrown.hasMessage ? formatReceived('Received message: ', thrown, 'message') + formatStack(thrown) : formatReceived('Received value:   ', thrown, 'value'));
  return {
    message,
    pass
  };
};
const toThrowExpectedAsymmetric = (matcherName, options, thrown, expected) => {
  const pass = thrown !== null && expected.asymmetricMatch(thrown.value);
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + formatExpected('Expected asymmetric matcher: not ', expected) + '\n' + (thrown !== null && thrown.hasMessage ? formatReceived('Received name:    ', thrown, 'name') + formatReceived('Received message: ', thrown, 'message') + formatStack(thrown) : formatReceived('Thrown value: ', thrown, 'value')) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + formatExpected('Expected asymmetric matcher: ', expected) + '\n' + (thrown === null ? DID_NOT_THROW : thrown.hasMessage ? formatReceived('Received name:    ', thrown, 'name') + formatReceived('Received message: ', thrown, 'message') + formatStack(thrown) : formatReceived('Thrown value: ', thrown, 'value'));
  return {
    message,
    pass
  };
};
const toThrowExpectedObject = (matcherName, options, thrown, expected) => {
  const expectedMessageAndCause = createMessageAndCause(expected);
  const thrownMessageAndCause = thrown === null ? null : createMessageAndCause(thrown.value);
  const isCompareErrorInstance = thrown?.isError && expected instanceof Error;
  const isExpectedCustomErrorInstance = expected.constructor.name !== Error.name;
  const pass = thrown !== null && thrown.message === expected.message && thrownMessageAndCause === expectedMessageAndCause && (!isCompareErrorInstance || !isExpectedCustomErrorInstance || thrown.value instanceof expected.constructor);
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + formatExpected(`Expected ${messageAndCause(expected)}: not `, expectedMessageAndCause) + (thrown !== null && thrown.hasMessage ? formatStack(thrown) : formatReceived('Received value:       ', thrown, 'value')) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (thrown === null ?
  // eslint-disable-next-line prefer-template
  formatExpected(`Expected ${messageAndCause(expected)}: `, expectedMessageAndCause) + '\n' + DID_NOT_THROW : thrown.hasMessage ?
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.printDiffOrStringify)(expectedMessageAndCause, thrownMessageAndCause, `Expected ${messageAndCause(expected)}`, `Received ${messageAndCause(thrown.value)}`, true) + '\n' + formatStack(thrown) : formatExpected(`Expected ${messageAndCause(expected)}: `, expectedMessageAndCause) + formatReceived('Received value:   ', thrown, 'value'));
  return {
    message,
    pass
  };
};
const toThrowExpectedClass = (matcherName, options, thrown, expected) => {
  const pass = thrown !== null && thrown.value instanceof expected;
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (0, _print.printExpectedConstructorNameNot)('Expected constructor', expected) + (thrown !== null && thrown.value != null && typeof thrown.value.constructor === 'function' && thrown.value.constructor !== expected ? (0, _print.printReceivedConstructorNameNot)('Received constructor', thrown.value.constructor, expected) : '') + '\n' + (thrown !== null && thrown.hasMessage ? formatReceived('Received message: ', thrown, 'message') + formatStack(thrown) : formatReceived('Received value: ', thrown, 'value')) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + (0, _print.printExpectedConstructorName)('Expected constructor', expected) + (thrown === null ? `\n${DID_NOT_THROW}` : `${thrown.value != null && typeof thrown.value.constructor === 'function' ? (0, _print.printReceivedConstructorName)('Received constructor', thrown.value.constructor) : ''}\n${thrown.hasMessage ? formatReceived('Received message: ', thrown, 'message') + formatStack(thrown) : formatReceived('Received value: ', thrown, 'value')}`);
  return {
    message,
    pass
  };
};
const toThrowExpectedString = (matcherName, options, thrown, expected) => {
  const pass = thrown !== null && thrown.message.includes(expected);
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + formatExpected('Expected substring: not ', expected) + (thrown !== null && thrown.hasMessage ? formatReceived('Received message:       ', thrown, 'message', expected) + formatStack(thrown) : formatReceived('Received value:         ', thrown, 'value')) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, undefined, options) + '\n\n' + formatExpected('Expected substring: ', expected) + (thrown === null ? `\n${DID_NOT_THROW}` : thrown.hasMessage ? formatReceived('Received message:   ', thrown, 'message') + formatStack(thrown) : formatReceived('Received value:     ', thrown, 'value'));
  return {
    message,
    pass
  };
};
const toThrow = (matcherName, options, thrown) => {
  const pass = thrown !== null;
  const message = pass ? () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + (thrown !== null && thrown.hasMessage ? formatReceived('Error name:    ', thrown, 'name') + formatReceived('Error message: ', thrown, 'message') + formatStack(thrown) : formatReceived('Thrown value: ', thrown, 'value')) : () =>
  // eslint-disable-next-line prefer-template
  (0, _jestMatcherUtils.matcherHint)(matcherName, undefined, '', options) + '\n\n' + DID_NOT_THROW;
  return {
    message,
    pass
  };
};
const formatExpected = (label, expected) => `${label + (0, _jestMatcherUtils.printExpected)(expected)}\n`;
const formatReceived = (label, thrown, key, expected) => {
  if (thrown === null) {
    return '';
  }
  if (key === 'message') {
    const message = thrown.message;
    if (typeof expected === 'string') {
      const index = message.indexOf(expected);
      if (index !== -1) {
        return `${label + (0, _print.printReceivedStringContainExpectedSubstring)(message, index, expected.length)}\n`;
      }
    } else if (expected instanceof RegExp) {
      return `${label + (0, _print.printReceivedStringContainExpectedResult)(message, typeof expected.exec === 'function' ? expected.exec(message) : null)}\n`;
    }
    return `${label + (0, _jestMatcherUtils.printReceived)(message)}\n`;
  }
  if (key === 'name') {
    return thrown.isError ? `${label + (0, _jestMatcherUtils.printReceived)(thrown.value.name)}\n` : '';
  }
  if (key === 'value') {
    return thrown.isError ? '' : `${label + (0, _jestMatcherUtils.printReceived)(thrown.value)}\n`;
  }
  return '';
};
const formatStack = thrown => {
  if (thrown === null || !thrown.isError) {
    return '';
  } else {
    const config = {
      rootDir: process.cwd(),
      testMatch: []
    };
    const options = {
      noStackTrace: false
    };
    if (thrown.value instanceof AggregateError) {
      return (0, _jestMessageUtil.formatExecError)(thrown.value, config, options);
    } else {
      return (0, _jestMessageUtil.formatStackTrace)((0, _jestMessageUtil.separateMessageFromStack)(thrown.value.stack).stack, config, options);
    }
  }
};
function createMessageAndCause(error) {
  if (error.cause) {
    const seen = new WeakSet();
    return JSON.stringify(buildSerializeError(error), (_, value) => {
      if (isObject(value)) {
        if (seen.has(value)) return;
        seen.add(value); // stop circular references
      }
      if (typeof value === 'bigint' || value === undefined) {
        return String(value);
      }
      return value;
    });
  }
  return error.message;
}
function buildSerializeError(error) {
  if (!isObject(error)) {
    return error;
  }
  const result = {};
  for (const name of Object.getOwnPropertyNames(error).sort()) {
    if (['stack', 'fileName', 'lineNumber'].includes(name)) {
      continue;
    }
    if (name === 'cause') {
      result[name] = buildSerializeError(error['cause']);
      continue;
    }
    result[name] = error[name];
  }
  return result;
}
function isObject(obj) {
  return obj != null && typeof obj === 'object';
}
function messageAndCause(error) {
  return error.cause === undefined ? 'message' : 'message and cause';
}
var _default = exports["default"] = matchers;

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
Object.defineProperty(exports, "AsymmetricMatcher", ({
  enumerable: true,
  get: function () {
    return _asymmetricMatchers.AsymmetricMatcher;
  }
}));
exports.expect = exports["default"] = exports.JestAssertionError = void 0;
var _expectUtils = require("@jest/expect-utils");
var matcherUtils = _interopRequireWildcard(require("jest-matcher-utils"));
var _jestUtil = require("jest-util");
var _asymmetricMatchers = __webpack_require__("./src/asymmetricMatchers.ts");
var _extractExpectedAssertionsErrors = _interopRequireDefault(__webpack_require__("./src/extractExpectedAssertionsErrors.ts"));
var _jestMatchersObject = __webpack_require__("./src/jestMatchersObject.ts");
var _matchers = _interopRequireDefault(__webpack_require__("./src/matchers.ts"));
var _spyMatchers = _interopRequireDefault(__webpack_require__("./src/spyMatchers.ts"));
var _toThrowMatchers = _interopRequireWildcard(__webpack_require__("./src/toThrowMatchers.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

/* eslint-disable local/prefer-spread-eventually */

class JestAssertionError extends Error {
  matcherResult;
}
exports.JestAssertionError = JestAssertionError;
const createToThrowErrorMatchingSnapshotMatcher = function (matcher) {
  return function (received, testNameOrInlineSnapshot) {
    return matcher.apply(this, [received, testNameOrInlineSnapshot, true]);
  };
};
const getPromiseMatcher = (name, matcher) => {
  if (name === 'toThrow') {
    return (0, _toThrowMatchers.createMatcher)(name, true);
  } else if (name === 'toThrowErrorMatchingSnapshot' || name === 'toThrowErrorMatchingInlineSnapshot') {
    return createToThrowErrorMatchingSnapshotMatcher(matcher);
  }
  return null;
};
const expect = (actual, ...rest) => {
  if (rest.length > 0) {
    throw new Error('Expect takes at most one argument.');
  }
  const allMatchers = (0, _jestMatchersObject.getMatchers)();
  const expectation = {
    not: {},
    rejects: {
      not: {}
    },
    resolves: {
      not: {}
    }
  };
  const err = new JestAssertionError();
  for (const name of Object.keys(allMatchers)) {
    const matcher = allMatchers[name];
    const promiseMatcher = getPromiseMatcher(name, matcher) || matcher;
    expectation[name] = makeThrowingMatcher(matcher, false, '', actual);
    expectation.not[name] = makeThrowingMatcher(matcher, true, '', actual);
    expectation.resolves[name] = makeResolveMatcher(name, promiseMatcher, false, actual, err);
    expectation.resolves.not[name] = makeResolveMatcher(name, promiseMatcher, true, actual, err);
    expectation.rejects[name] = makeRejectMatcher(name, promiseMatcher, false, actual, err);
    expectation.rejects.not[name] = makeRejectMatcher(name, promiseMatcher, true, actual, err);
  }
  return expectation;
};
exports.expect = expect;
const getMessage = message => message && message() || matcherUtils.RECEIVED_COLOR('No message was specified for this matcher.');
const makeResolveMatcher = (matcherName, matcher, isNot, actual, outerErr) => (...args) => {
  const options = {
    isNot,
    promise: 'resolves'
  };
  const actualWrapper = typeof actual === 'function' ? actual() : actual;
  if (!(0, _jestUtil.isPromise)(actualWrapper)) {
    throw new JestAssertionError(matcherUtils.matcherErrorMessage(matcherUtils.matcherHint(matcherName, undefined, '', options), `${matcherUtils.RECEIVED_COLOR('received')} value must be a promise or a function returning a promise`, matcherUtils.printWithType('Received', actual, matcherUtils.printReceived)));
  }
  const innerErr = new JestAssertionError();
  return actualWrapper.then(result => makeThrowingMatcher(matcher, isNot, 'resolves', result, innerErr).apply(null, args), error => {
    outerErr.message = `${matcherUtils.matcherHint(matcherName, undefined, '', options)}\n\n` + 'Received promise rejected instead of resolved\n' + `Rejected to value: ${matcherUtils.printReceived(error)}`;
    throw outerErr;
  });
};
const makeRejectMatcher = (matcherName, matcher, isNot, actual, outerErr) => (...args) => {
  const options = {
    isNot,
    promise: 'rejects'
  };
  const actualWrapper = typeof actual === 'function' ? actual() : actual;
  if (!(0, _jestUtil.isPromise)(actualWrapper)) {
    throw new JestAssertionError(matcherUtils.matcherErrorMessage(matcherUtils.matcherHint(matcherName, undefined, '', options), `${matcherUtils.RECEIVED_COLOR('received')} value must be a promise or a function returning a promise`, matcherUtils.printWithType('Received', actual, matcherUtils.printReceived)));
  }
  const innerErr = new JestAssertionError();
  return actualWrapper.then(result => {
    outerErr.message = `${matcherUtils.matcherHint(matcherName, undefined, '', options)}\n\n` + 'Received promise resolved instead of rejected\n' + `Resolved to value: ${matcherUtils.printReceived(result)}`;
    throw outerErr;
  }, error => makeThrowingMatcher(matcher, isNot, 'rejects', error, innerErr).apply(null, args));
};
const makeThrowingMatcher = (matcher, isNot, promise, actual, err) => function throwingMatcher(...args) {
  let throws = true;
  const utils = {
    ...matcherUtils,
    iterableEquality: _expectUtils.iterableEquality,
    subsetEquality: _expectUtils.subsetEquality
  };
  const matcherUtilsThing = {
    customTesters: (0, _jestMatchersObject.getCustomEqualityTesters)(),
    // When throws is disabled, the matcher will not throw errors during test
    // execution but instead add them to the global matcher state. If a
    // matcher throws, test execution is normally stopped immediately. The
    // snapshot matcher uses it because we want to log all snapshot
    // failures in a test.
    dontThrow: () => throws = false,
    equals: _expectUtils.equals,
    utils
  };
  const matcherContext = {
    ...(0, _jestMatchersObject.getState)(),
    ...matcherUtilsThing,
    error: err,
    isNot,
    promise
  };
  const processResult = (result, asyncError) => {
    _validateResult(result);
    (0, _jestMatchersObject.getState)().assertionCalls++;
    if (result.pass && isNot || !result.pass && !isNot) {
      // XOR
      const message = getMessage(result.message);
      let error;
      if (err) {
        error = err;
        error.message = message;
      } else if (asyncError) {
        error = asyncError;
        error.message = message;
      } else {
        error = new JestAssertionError(message);

        // Try to remove this function from the stack trace frame.
        // Guard for some environments (browsers) that do not support this feature.
        if (Error.captureStackTrace) {
          Error.captureStackTrace(error, throwingMatcher);
        }
      }
      // Passing the result of the matcher with the error so that a custom
      // reporter could access the actual and expected objects of the result
      // for example in order to display a custom visual diff
      error.matcherResult = {
        ...result,
        message
      };
      if (throws) {
        throw error;
      } else {
        (0, _jestMatchersObject.getState)().suppressedErrors.push(error);
      }
    } else {
      (0, _jestMatchersObject.getState)().numPassingAsserts++;
    }
  };
  const handleError = error => {
    if (matcher[_jestMatchersObject.INTERNAL_MATCHER_FLAG] === true && !(error instanceof JestAssertionError) && error.name !== 'PrettyFormatPluginError' &&
    // Guard for some environments (browsers) that do not support this feature.
    Error.captureStackTrace) {
      // Try to remove this and deeper functions from the stack trace frame.
      Error.captureStackTrace(error, throwingMatcher);
    }
    throw error;
  };
  let potentialResult;
  try {
    potentialResult = matcher[_jestMatchersObject.INTERNAL_MATCHER_FLAG] === true ? matcher.call(matcherContext, actual, ...args) :
    // It's a trap specifically for inline snapshot to capture this name
    // in the stack trace, so that it can correctly get the custom matcher
    // function call.
    function __EXTERNAL_MATCHER_TRAP__() {
      return matcher.call(matcherContext, actual, ...args);
    }();
    if ((0, _jestUtil.isPromise)(potentialResult)) {
      const asyncError = new JestAssertionError();
      if (Error.captureStackTrace) {
        Error.captureStackTrace(asyncError, throwingMatcher);
      }
      return potentialResult.then(aResult => processResult(aResult, asyncError)).catch(handleError);
    } else {
      return processResult(potentialResult);
    }
  } catch (error) {
    return handleError(error);
  }
};
expect.extend = matchers => (0, _jestMatchersObject.setMatchers)(matchers, false, expect);
expect.addEqualityTesters = customTesters => (0, _jestMatchersObject.addCustomEqualityTesters)(customTesters);
expect.anything = _asymmetricMatchers.anything;
expect.any = _asymmetricMatchers.any;
expect.not = {
  arrayContaining: _asymmetricMatchers.arrayNotContaining,
  arrayOf: _asymmetricMatchers.notArrayOf,
  closeTo: _asymmetricMatchers.notCloseTo,
  objectContaining: _asymmetricMatchers.objectNotContaining,
  stringContaining: _asymmetricMatchers.stringNotContaining,
  stringMatching: _asymmetricMatchers.stringNotMatching
};
expect.arrayContaining = _asymmetricMatchers.arrayContaining;
expect.arrayOf = _asymmetricMatchers.arrayOf;
expect.closeTo = _asymmetricMatchers.closeTo;
expect.objectContaining = _asymmetricMatchers.objectContaining;
expect.stringContaining = _asymmetricMatchers.stringContaining;
expect.stringMatching = _asymmetricMatchers.stringMatching;
const _validateResult = result => {
  if (typeof result !== 'object' || typeof result.pass !== 'boolean' || result.message && typeof result.message !== 'string' && typeof result.message !== 'function') {
    throw new Error('Unexpected return from a matcher function.\n' + 'Matcher functions should ' + 'return an object in the following format:\n' + '  {message?: string | function, pass: boolean}\n' + `'${matcherUtils.stringify(result)}' was returned`);
  }
};
function assertions(expected) {
  const error = new _jestUtil.ErrorWithStack(undefined, assertions);
  (0, _jestMatchersObject.setState)({
    expectedAssertionsNumber: expected,
    expectedAssertionsNumberError: error
  });
}
function hasAssertions(...args) {
  const error = new _jestUtil.ErrorWithStack(undefined, hasAssertions);
  matcherUtils.ensureNoExpected(args[0], '.hasAssertions');
  (0, _jestMatchersObject.setState)({
    isExpectingAssertions: true,
    isExpectingAssertionsError: error
  });
}

// add default jest matchers
(0, _jestMatchersObject.setMatchers)(_matchers.default, true, expect);
(0, _jestMatchersObject.setMatchers)(_spyMatchers.default, true, expect);
(0, _jestMatchersObject.setMatchers)(_toThrowMatchers.default, true, expect);
expect.assertions = assertions;
expect.hasAssertions = hasAssertions;
expect.getState = _jestMatchersObject.getState;
expect.setState = _jestMatchersObject.setState;
expect.extractExpectedAssertionsErrors = _extractExpectedAssertionsErrors.default;
var _default = exports["default"] = expect;
})();

module.exports = __webpack_exports__;
/******/ })()
;