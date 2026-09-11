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

/***/ "./src/immutableUtils.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.isImmutableList = isImmutableList;
exports.isImmutableOrderedKeyed = isImmutableOrderedKeyed;
exports.isImmutableOrderedSet = isImmutableOrderedSet;
exports.isImmutableRecord = isImmutableRecord;
exports.isImmutableUnorderedKeyed = isImmutableUnorderedKeyed;
exports.isImmutableUnorderedSet = isImmutableUnorderedSet;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

// SENTINEL constants are from https://github.com/immutable-js/immutable-js/tree/main/src/predicates
const IS_KEYED_SENTINEL = '@@__IMMUTABLE_KEYED__@@';
const IS_SET_SENTINEL = '@@__IMMUTABLE_SET__@@';
const IS_LIST_SENTINEL = '@@__IMMUTABLE_LIST__@@';
const IS_ORDERED_SENTINEL = '@@__IMMUTABLE_ORDERED__@@';
const IS_RECORD_SYMBOL = '@@__IMMUTABLE_RECORD__@@';
function isObjectLiteral(source) {
  return source != null && typeof source === 'object' && !Array.isArray(source);
}
function isImmutableUnorderedKeyed(source) {
  return Boolean(source && isObjectLiteral(source) && source[IS_KEYED_SENTINEL] && !source[IS_ORDERED_SENTINEL]);
}
function isImmutableUnorderedSet(source) {
  return Boolean(source && isObjectLiteral(source) && source[IS_SET_SENTINEL] && !source[IS_ORDERED_SENTINEL]);
}
function isImmutableList(source) {
  return Boolean(source && isObjectLiteral(source) && source[IS_LIST_SENTINEL]);
}
function isImmutableOrderedKeyed(source) {
  return Boolean(source && isObjectLiteral(source) && source[IS_KEYED_SENTINEL] && source[IS_ORDERED_SENTINEL]);
}
function isImmutableOrderedSet(source) {
  return Boolean(source && isObjectLiteral(source) && source[IS_SET_SENTINEL] && source[IS_ORDERED_SENTINEL]);
}
function isImmutableRecord(source) {
  return Boolean(source && isObjectLiteral(source) && source[IS_RECORD_SYMBOL]);
}

/***/ },

/***/ "./src/index.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
var _exportNames = {
  equals: true,
  isA: true
};
Object.defineProperty(exports, "equals", ({
  enumerable: true,
  get: function () {
    return _jasmineUtils.equals;
  }
}));
Object.defineProperty(exports, "isA", ({
  enumerable: true,
  get: function () {
    return _jasmineUtils.isA;
  }
}));
var _jasmineUtils = __webpack_require__("./src/jasmineUtils.ts");
var _utils = __webpack_require__("./src/utils.ts");
Object.keys(_utils).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (Object.prototype.hasOwnProperty.call(_exportNames, key)) return;
  if (key in exports && exports[key] === _utils[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _utils[key];
    }
  });
});

/***/ },

/***/ "./src/jasmineUtils.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.equals = void 0;
exports.isA = isA;
/*
Copyright (c) 2008-2016 Pivotal Labs

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

*/

// Extracted out of jasmine 2.5.2
const equals = (a, b, customTesters, strictCheck) => {
  customTesters = customTesters || [];
  return eq(a, b, [], [], customTesters, strictCheck);
};
exports.equals = equals;
function isAsymmetric(obj) {
  return !!obj && isA('Function', obj.asymmetricMatch);
}
function asymmetricMatch(a, b) {
  const asymmetricA = isAsymmetric(a);
  const asymmetricB = isAsymmetric(b);
  if (asymmetricA && asymmetricB) {
    return undefined;
  }
  if (asymmetricA) {
    return a.asymmetricMatch(b);
  }
  if (asymmetricB) {
    return b.asymmetricMatch(a);
  }
}

// Equality function lovingly adapted from isEqual in
//   [Underscore](http://underscorejs.org)
function eq(a, b, aStack, bStack, customTesters, strictCheck) {
  let result = true;
  const asymmetricResult = asymmetricMatch(a, b);
  if (asymmetricResult !== undefined) {
    return asymmetricResult;
  }
  const testerContext = {
    equals
  };
  for (const item of customTesters) {
    const customTesterResult = item.call(testerContext, a, b, customTesters);
    if (customTesterResult !== undefined) {
      return customTesterResult;
    }
  }
  if (a instanceof Error && b instanceof Error) {
    return a.message === b.message;
  }
  if (Object.is(a, b)) {
    return true;
  }
  // A strict comparison is necessary because `null == undefined`.
  if (a === null || b === null) {
    return false;
  }
  const className = Object.prototype.toString.call(a);
  if (className !== Object.prototype.toString.call(b)) {
    return false;
  }
  switch (className) {
    case '[object Boolean]':
    case '[object String]':
    case '[object Number]':
      if (typeof a !== typeof b) {
        // One is a primitive, one a `new Primitive()`
        return false;
      } else if (typeof a !== 'object' && typeof b !== 'object') {
        // both are proper primitives
        return false;
      } else {
        // both are `new Primitive()`s
        return Object.is(a.valueOf(), b.valueOf());
      }
    case '[object Date]':
      // Coerce dates to numeric primitive values. Dates are compared by their
      // millisecond representations. Note that invalid dates with millisecond representations
      // of `NaN` are not equivalent.
      return +a === +b;
    // RegExps are compared by their source patterns and flags.
    case '[object RegExp]':
      return a.source === b.source && a.flags === b.flags;
    // URLs are compared by their href property which contains the entire url string.
    case '[object URL]':
      return a.href === b.href;
  }
  if (typeof a !== 'object' || typeof b !== 'object') {
    return false;
  }

  // Use DOM3 method isEqualNode (IE>=9)
  if (isDomNode(a) && isDomNode(b)) {
    return a.isEqualNode(b);
  }

  // Used to detect circular references.
  let length = aStack.length;
  while (length--) {
    // Linear search. Performance is inversely proportional to the number of
    // unique nested structures.
    // circular references at same depth are equal
    // circular reference is not equal to non-circular one
    if (aStack[length] === a) {
      return bStack[length] === b;
    } else if (bStack[length] === b) {
      return false;
    }
  }
  // Add the first object to the stack of traversed objects.
  aStack.push(a);
  bStack.push(b);
  // Recursively compare objects and arrays.
  // Compare array lengths to determine if a deep comparison is necessary.
  if (strictCheck && className === '[object Array]' && a.length !== b.length) {
    return false;
  }

  // Deep compare objects.
  const aKeys = keys(a, hasKey);
  let key;
  const bKeys = keys(b, hasKey);
  // Add keys corresponding to asymmetric matchers if they miss in non strict check mode
  if (!strictCheck) {
    for (let index = 0; index !== bKeys.length; ++index) {
      key = bKeys[index];
      if ((isAsymmetric(b[key]) || b[key] === undefined) && !hasKey(a, key)) {
        aKeys.push(key);
      }
    }
    for (let index = 0; index !== aKeys.length; ++index) {
      key = aKeys[index];
      if ((isAsymmetric(a[key]) || a[key] === undefined) && !hasKey(b, key)) {
        bKeys.push(key);
      }
    }
  }

  // Ensure that both objects contain the same number of properties before comparing deep equality.
  let size = aKeys.length;
  if (bKeys.length !== size) {
    return false;
  }
  while (size--) {
    key = aKeys[size];

    // Deep compare each member
    if (strictCheck) result = hasKey(b, key) && eq(a[key], b[key], aStack, bStack, customTesters, strictCheck);else result = (hasKey(b, key) || isAsymmetric(a[key]) || a[key] === undefined) && eq(a[key], b[key], aStack, bStack, customTesters, strictCheck);
    if (!result) {
      return false;
    }
  }
  // Remove the first object from the stack of traversed objects.
  aStack.pop();
  bStack.pop();
  return result;
}
function keys(obj, hasKey) {
  const keys = [];
  for (const key in obj) {
    if (hasKey(obj, key)) {
      keys.push(key);
    }
  }
  return [...keys, ...Object.getOwnPropertySymbols(obj).filter(symbol => Object.getOwnPropertyDescriptor(obj, symbol).enumerable)];
}
function hasKey(obj, key) {
  return Object.prototype.hasOwnProperty.call(obj, key);
}
function isA(typeName, value) {
  return Object.prototype.toString.apply(value) === `[object ${typeName}]`;
}
function isDomNode(obj) {
  return obj !== null && typeof obj === 'object' && typeof obj.nodeType === 'number' && typeof obj.nodeName === 'string' && typeof obj.isEqualNode === 'function';
}

/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.arrayBufferEquality = void 0;
exports.emptyObject = emptyObject;
exports.typeEquality = exports.subsetEquality = exports.sparseArrayEquality = exports.pathAsArray = exports.partition = exports.iterableEquality = exports.isOneline = exports.isError = exports.getPath = exports.getObjectSubset = exports.getObjectKeys = void 0;
var _getType = require("@jest/get-type");
var _immutableUtils = __webpack_require__("./src/immutableUtils.ts");
var _jasmineUtils = __webpack_require__("./src/jasmineUtils.ts");
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */
/**
 * Checks if `hasOwnProperty(object, key)` up the prototype chain, stopping at `Object.prototype`.
 */
const hasPropertyInObject = (object, key) => {
  const shouldTerminate = !object || typeof object !== 'object' || object === Object.prototype;
  if (shouldTerminate) {
    return false;
  }
  return Object.prototype.hasOwnProperty.call(object, key) || hasPropertyInObject(Object.getPrototypeOf(object), key);
};

// Retrieves an object's keys for evaluation by getObjectSubset.  This evaluates
// the prototype chain for string keys but not for non-enumerable symbols.
// (Otherwise, it could find values such as a Set or Map's Symbol.toStringTag,
// with unexpected results.)
const getObjectKeys = object => {
  return [...Object.keys(object), ...Object.getOwnPropertySymbols(object).filter(s => Object.getOwnPropertyDescriptor(object, s)?.enumerable)];
};
exports.getObjectKeys = getObjectKeys;
const getPath = (object, propertyPath) => {
  if (!Array.isArray(propertyPath)) {
    propertyPath = pathAsArray(propertyPath);
  }
  if (propertyPath.length > 0) {
    const lastProp = propertyPath.length === 1;
    const prop = propertyPath[0];
    const newObject = object[prop];
    if (!lastProp && (newObject === null || newObject === undefined)) {
      // This is not the last prop in the chain. If we keep recursing it will
      // hit a `can't access property X of undefined | null`. At this point we
      // know that the chain has broken and we can return right away.
      return {
        hasEndProp: false,
        lastTraversedObject: object,
        traversedPath: []
      };
    }
    const result = getPath(newObject, propertyPath.slice(1));
    if (result.lastTraversedObject === null) {
      result.lastTraversedObject = object;
    }
    result.traversedPath.unshift(prop);
    if (lastProp) {
      // Does object have the property with an undefined value?
      // Although primitive values support bracket notation (above)
      // they would throw TypeError for in operator (below).
      result.endPropIsDefined = !(0, _getType.isPrimitive)(object) && prop in object;
      result.hasEndProp = newObject !== undefined || result.endPropIsDefined;
      if (!result.hasEndProp) {
        result.traversedPath.shift();
      }
    }
    return result;
  }
  return {
    lastTraversedObject: null,
    traversedPath: [],
    value: object
  };
};

// Strip properties from object that are not present in the subset. Useful for
// printing the diff for toMatchObject() without adding unrelated noise.
/* eslint-disable @typescript-eslint/explicit-module-boundary-types */
exports.getPath = getPath;
const getObjectSubset = (object, subset, customTesters = [], seenReferences = new WeakMap()) => {
  /* eslint-enable @typescript-eslint/explicit-module-boundary-types */
  if (Array.isArray(object)) {
    if (Array.isArray(subset) && subset.length === object.length) {
      // The map method returns correct subclass of subset.
      return subset.map((sub, i) => getObjectSubset(object[i], sub, customTesters));
    }
  } else if (object instanceof Date) {
    return object;
  } else if (isObject(object) && isObject(subset)) {
    if ((0, _jasmineUtils.equals)(object, subset, [...customTesters, iterableEquality, subsetEquality])) {
      // Avoid unnecessary copy which might return Object instead of subclass.
      return subset;
    }
    const trimmed = {};
    seenReferences.set(object, trimmed);
    for (const key of getObjectKeys(object)) {
      if (!hasPropertyInObject(subset, key)) {
        continue;
      }
      trimmed[key] = seenReferences.has(object[key]) ? seenReferences.get(object[key]) : getObjectSubset(object[key], subset[key], customTesters, seenReferences);
    }
    if (getObjectKeys(trimmed).length > 0) {
      return trimmed;
    }
  }
  return object;
};
exports.getObjectSubset = getObjectSubset;
const IteratorSymbol = Symbol.iterator;
const hasIterator = object => !!(object != null && object[IteratorSymbol]);

/* eslint-disable @typescript-eslint/explicit-module-boundary-types */
const iterableEquality = (a, b, customTesters = [], /* eslint-enable @typescript-eslint/explicit-module-boundary-types */
aStack = [], bStack = []) => {
  if (typeof a !== 'object' || typeof b !== 'object' || Array.isArray(a) || Array.isArray(b) || ArrayBuffer.isView(a) || ArrayBuffer.isView(b) || !hasIterator(a) || !hasIterator(b)) {
    return undefined;
  }
  if (a.constructor !== b.constructor) {
    // Same cross-realm constructor check as typeEquality — see #14011.
    // https://github.com/jestjs/jest/issues/14011
    if (a.constructor == null || b.constructor == null || a.constructor.name !== b.constructor.name || !isNativeFunction(a.constructor) || !isNativeFunction(b.constructor)) {
      return false;
    }
  }
  let length = aStack.length;
  while (length--) {
    // Linear search. Performance is inversely proportional to the number of
    // unique nested structures.
    // circular references at same depth are equal
    // circular reference is not equal to non-circular one
    if (aStack[length] === a) {
      return bStack[length] === b;
    }
  }
  aStack.push(a);
  bStack.push(b);
  const iterableEqualityWithStack = (a, b) => iterableEquality(a, b, [...filteredCustomTesters], [...aStack], [...bStack]);

  // Replace any instance of iterableEquality with the new
  // iterableEqualityWithStack so we can do circular detection
  const filteredCustomTesters = [...customTesters.filter(t => t !== iterableEquality), iterableEqualityWithStack];
  if (a.size !== undefined) {
    if (a.size !== b.size) {
      return false;
    } else if ((0, _jasmineUtils.isA)('Set', a) || (0, _immutableUtils.isImmutableUnorderedSet)(a)) {
      let allFound = true;
      for (const aValue of a) {
        if (!b.has(aValue)) {
          let has = false;
          for (const bValue of b) {
            const isEqual = (0, _jasmineUtils.equals)(aValue, bValue, filteredCustomTesters);
            if (isEqual === true) {
              has = true;
            }
          }
          if (has === false) {
            allFound = false;
            break;
          }
        }
      }
      // Remove the first value from the stack of traversed values.
      aStack.pop();
      bStack.pop();
      return allFound;
    } else if ((0, _jasmineUtils.isA)('Map', a) || (0, _immutableUtils.isImmutableUnorderedKeyed)(a)) {
      let allFound = true;
      for (const aEntry of a) {
        if (!b.has(aEntry[0]) || !(0, _jasmineUtils.equals)(aEntry[1], b.get(aEntry[0]), filteredCustomTesters)) {
          let has = false;
          for (const bEntry of b) {
            const matchedKey = (0, _jasmineUtils.equals)(aEntry[0], bEntry[0], filteredCustomTesters);
            let matchedValue = false;
            if (matchedKey === true) {
              matchedValue = (0, _jasmineUtils.equals)(aEntry[1], bEntry[1], filteredCustomTesters);
            }
            if (matchedValue === true) {
              has = true;
            }
          }
          if (has === false) {
            allFound = false;
            break;
          }
        }
      }
      // Remove the first value from the stack of traversed values.
      aStack.pop();
      bStack.pop();
      return allFound;
    }
  }
  let aIterator;
  let bIterator;
  try {
    aIterator = a[IteratorSymbol]();
    bIterator = b[IteratorSymbol]();
  } catch {
    // If the iterator factory itself throws (e.g. a TypedArray method used as
    // [Symbol.iterator] on a plain object), we cannot compare as iterables.
    // Return undefined so equals() falls through to Object.is / property checks.
    aStack.pop();
    bStack.pop();
    return undefined;
  }
  let aStep = aIterator.next();
  while (!aStep.done) {
    const bStep = bIterator.next();
    if (bStep.done || !(0, _jasmineUtils.equals)(aStep.value, bStep.value, filteredCustomTesters)) {
      return false;
    }
    aStep = aIterator.next();
  }
  if (!bIterator.next().done) {
    return false;
  }
  if (!(0, _immutableUtils.isImmutableList)(a) && !(0, _immutableUtils.isImmutableOrderedKeyed)(a) && !(0, _immutableUtils.isImmutableOrderedSet)(a) && !(0, _immutableUtils.isImmutableRecord)(a)) {
    const aEntries = entries(a);
    const bEntries = entries(b);
    if (!(0, _jasmineUtils.equals)(aEntries, bEntries)) {
      return false;
    }
  }

  // Remove the first value from the stack of traversed values.
  aStack.pop();
  bStack.pop();
  return true;
};
exports.iterableEquality = iterableEquality;
const entries = obj => {
  if (!isObject(obj)) return [];
  const symbolProperties = Object.getOwnPropertySymbols(obj).filter(key => key !== Symbol.iterator).map(key => [key, obj[key]]);
  return [...symbolProperties, ...Object.entries(obj)];
};
const isObject = a => a !== null && typeof a === 'object';
const isObjectWithKeys = a => isObject(a) && !(a instanceof Error) && !Array.isArray(a) && !(a instanceof Date) && !(a instanceof Set) && !(a instanceof Map);
const subsetEquality = (object, subset, customTesters = []) => {
  const filteredCustomTesters = customTesters.filter(t => t !== subsetEquality);

  // subsetEquality needs to keep track of the references
  // it has already visited to avoid infinite loops in case
  // there are circular references in the subset passed to it.
  const subsetEqualityWithContext = (seenReferences = new WeakMap()) => (object, subset) => {
    if (!isObjectWithKeys(subset)) {
      return undefined;
    }
    if (seenReferences.has(subset)) return undefined;
    seenReferences.set(subset, true);
    const matchResult = getObjectKeys(subset).every(key => {
      if (isObjectWithKeys(subset[key])) {
        if (seenReferences.has(subset[key])) {
          return (0, _jasmineUtils.equals)(object[key], subset[key], filteredCustomTesters);
        }
      }
      const result = object != null && hasPropertyInObject(object, key) && (0, _jasmineUtils.equals)(object[key], subset[key], [...filteredCustomTesters, subsetEqualityWithContext(seenReferences)]);
      // The main goal of using seenReference is to avoid circular node on tree.
      // It will only happen within a parent and its child, not a node and nodes next to it (same level)
      // We should keep the reference for a parent and its child only
      // Thus we should delete the reference immediately so that it doesn't interfere
      // other nodes within the same level on tree.
      seenReferences.delete(subset[key]);
      return result;
    });
    seenReferences.delete(subset);
    return matchResult;
  };
  return subsetEqualityWithContext()(object, subset);
};

// Returns true if `fn` is a native function (its toString contains "[native code]").
exports.subsetEquality = subsetEquality;
function isNativeFunction(fn) {
  return typeof fn === 'function' && Function.prototype.toString.call(fn).includes('[native code]');
}

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
const typeEquality = (a, b) => {
  if (a == null || b == null || a.constructor === b.constructor ||
  // Since Jest globals are different from Node globals,
  // constructors are different even between arrays when comparing properties of mock objects.
  // Both of them should be able to compare correctly when they are array-to-array.
  // https://github.com/jestjs/jest/issues/2549
  Array.isArray(a) && Array.isArray(b)) {
    return undefined;
  }

  // structuredClone (and other cross-realm calls) return objects whose
  // constructors come from a different VM context, so identity checks fail.
  // Fall back to comparing constructor names for native built-ins only —
  // user-defined classes still need identity equality.
  // https://github.com/jestjs/jest/issues/14011
  if (a.constructor != null && b.constructor != null && a.constructor.name === b.constructor.name && isNativeFunction(a.constructor) && isNativeFunction(b.constructor)) {
    return undefined;
  }
  return false;
};
exports.typeEquality = typeEquality;
const arrayBufferEquality = (a, b) => {
  let dataViewA = a;
  let dataViewB = b;
  if (isArrayBuffer(a) && isArrayBuffer(b)) {
    dataViewA = new DataView(a);
    dataViewB = new DataView(b);
  } else if (ArrayBuffer.isView(a) && ArrayBuffer.isView(b)) {
    dataViewA = new DataView(a.buffer, a.byteOffset, a.byteLength);
    dataViewB = new DataView(b.buffer, b.byteOffset, b.byteLength);
  }
  if (!(dataViewA instanceof DataView && dataViewB instanceof DataView)) {
    return undefined;
  }

  // Buffers are not equal when they do not have the same byte length
  if (dataViewA.byteLength !== dataViewB.byteLength) {
    return false;
  }

  // Check if every byte value is equal to each other
  for (let i = 0; i < dataViewA.byteLength; i++) {
    if (dataViewA.getUint8(i) !== dataViewB.getUint8(i)) {
      return false;
    }
  }
  return true;
};
exports.arrayBufferEquality = arrayBufferEquality;
function isArrayBuffer(obj) {
  return Object.prototype.toString.call(obj) === '[object ArrayBuffer]';
}
const sparseArrayEquality = (a, b, customTesters = []) => {
  if (!Array.isArray(a) || !Array.isArray(b)) {
    return undefined;
  }

  // A sparse array [, , 1] will have keys ["2"] whereas [undefined, undefined, 1] will have keys ["0", "1", "2"]
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);
  return (0, _jasmineUtils.equals)(a, b, customTesters.filter(t => t !== sparseArrayEquality), true) && (0, _jasmineUtils.equals)(aKeys, bKeys);
};
exports.sparseArrayEquality = sparseArrayEquality;
const partition = (items, predicate) => {
  const result = [[], []];
  for (const item of items) result[predicate(item) ? 0 : 1].push(item);
  return result;
};
exports.partition = partition;
const pathAsArray = propertyPath => {
  const properties = [];
  if (propertyPath === '') {
    properties.push('');
    return properties;
  }

  // will match everything that's not a dot or a bracket, and "" for consecutive dots.
  const pattern = new RegExp('[^.[\\]]+|(?=(?:\\.)(?:\\.|$))', 'g');

  // Because the regex won't match a dot in the beginning of the path, if present.
  if (propertyPath[0] === '.') {
    properties.push('');
  }
  propertyPath.replaceAll(pattern, match => {
    properties.push(match);
    return match;
  });
  return properties;
};

// Copied from https://github.com/graingert/angular.js/blob/a43574052e9775cbc1d7dd8a086752c979b0f020/src/Angular.js#L685-L693
exports.pathAsArray = pathAsArray;
const isError = value => {
  switch (Object.prototype.toString.call(value)) {
    case '[object Error]':
    case '[object Exception]':
    case '[object DOMException]':
      return true;
    default:
      return value instanceof Error;
  }
};
exports.isError = isError;
function emptyObject(obj) {
  return obj && typeof obj === 'object' ? Object.keys(obj).length === 0 : false;
}
const MULTILINE_REGEXP = /[\n\r]/;
const isOneline = (expected, received) => typeof expected === 'string' && typeof received === 'string' && (!MULTILINE_REGEXP.test(expected) || !MULTILINE_REGEXP.test(received));
exports.isOneline = isOneline;

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