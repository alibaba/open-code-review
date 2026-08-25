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

/***/ "./src/ErrorWithStack.ts"
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

class ErrorWithStack extends Error {
  constructor(message, callsite, stackLimit) {
    // Ensure we have a large stack length so we get full details.
    const originalStackLimit = Error.stackTraceLimit;
    if (stackLimit) {
      Error.stackTraceLimit = Math.max(stackLimit, originalStackLimit || 10);
    }
    super(message);
    if (Error.captureStackTrace) {
      Error.captureStackTrace(this, callsite);
    }
    Error.stackTraceLimit = originalStackLimit;
  }
}
exports["default"] = ErrorWithStack;

/***/ },

/***/ "./src/clearLine.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = clearLine;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function clearLine(stream) {
  if (stream.isTTY) {
    stream.write('\u001B[999D\u001B[K');
  }
}

/***/ },

/***/ "./src/convertDescriptorToString.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = convertDescriptorToString;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function convertDescriptorToString(descriptor) {
  switch (typeof descriptor) {
    case 'function':
      if (descriptor.name) {
        return descriptor.name;
      }
      break;
    case 'number':
    case 'undefined':
      return `${descriptor}`;
    case 'string':
      return descriptor;
  }
  throw new Error(`Invalid first argument, ${descriptor}. It must be a named class, named function, number, or string.`);
}

/***/ },

/***/ "./src/createDirectory.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = createDirectory;
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
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

function createDirectory(path) {
  try {
    fs().mkdirSync(path, {
      recursive: true
    });
  } catch (error) {
    if (error.code !== 'EEXIST') {
      throw error;
    }
  }
}

/***/ },

/***/ "./src/createProcessObject.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = createProcessObject;
var _deepCyclicCopy = _interopRequireDefault(__webpack_require__("./src/deepCyclicCopy.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const BLACKLIST = new Set(['env', 'mainModule', '_events']);
const isWin32 = process.platform === 'win32';
const proto = Object.getPrototypeOf(process.env);

// The "process.env" object has a bunch of particularities: first, it does not
// directly extend from Object; second, it converts any assigned value to a
// string; and third, it is case-insensitive in Windows. We use a proxy here to
// mimic it (see https://nodejs.org/api/process.html#process_process_env).

function createProcessEnv() {
  const real = Object.create(proto);
  const lookup = {};
  function deletePropertyWin32(_target, key) {
    for (const name in real) {
      if (Object.prototype.hasOwnProperty.call(real, name)) {
        if (typeof key === 'string') {
          if (name.toLowerCase() === key.toLowerCase()) {
            delete real[name];
            delete lookup[name.toLowerCase()];
          }
        } else {
          if (key === name) {
            delete real[name];
            delete lookup[name];
          }
        }
      }
    }
    return true;
  }
  function deleteProperty(_target, key) {
    delete real[key];
    delete lookup[key];
    return true;
  }
  function getProperty(_target, key) {
    return real[key];
  }
  function getPropertyWin32(_target, key) {
    if (typeof key === 'string') {
      return lookup[key in proto ? key : key.toLowerCase()];
    } else {
      return real[key];
    }
  }
  const proxy = new Proxy(real, {
    deleteProperty: isWin32 ? deletePropertyWin32 : deleteProperty,
    get: isWin32 ? getPropertyWin32 : getProperty,
    set(_target, key, value) {
      const strValue = `${value}`;
      if (typeof key === 'string') {
        lookup[key.toLowerCase()] = strValue;
      }
      real[key] = strValue;
      return true;
    }
  });
  return Object.assign(proxy, process.env);
}
function createProcessObject() {
  const process = require('node:process');
  const newProcess = (0, _deepCyclicCopy.default)(process, {
    blacklist: BLACKLIST,
    keepPrototype: true
  });
  try {
    // This fails on Node 12, but it's already set to 'process'
    newProcess[Symbol.toStringTag] = 'process';
  } catch (error) {
    // Make sure it's actually set instead of potentially ignoring errors
    if (newProcess[Symbol.toStringTag] !== 'process') {
      error.message = `Unable to set toStringTag on process. Please open up an issue at https://github.com/jestjs/jest\n\n${error.message}`;
      throw error;
    }
  }

  // Sequentially execute all constructors over the object.
  let proto = process;
  while (proto = Object.getPrototypeOf(proto)) {
    if (typeof proto.constructor === 'function') {
      proto.constructor.call(newProcess);
    }
  }
  newProcess.env = createProcessEnv();
  newProcess.send = () => true;
  Object.defineProperty(newProcess, 'domain', {
    get() {
      return process.domain;
    }
  });

  // Ensure feature flags reflect Jest's capabilities inside the VM.
  // Node may expose `process.features.require_module` which signals that
  // requiring ESM via `require()` is supported. Jest's runtime does not
  // support requiring ESM modules through CJS `require`, so we override
  // the flag to false to allow defensive code paths to behave correctly.
  //
  const features = newProcess.features;
  if (features && typeof features === 'object') {
    // Only override if the host process exposes the flag
    if ('require_module' in features) {
      try {
        Object.defineProperty(features, 'require_module', {
          configurable: true,
          enumerable: true,
          get: () => false
        });
      } catch {
        // If redefining fails for any reason, fall back to direct assignment
        try {
          features.require_module = false;
        } catch {
          // ignore if we cannot override
        }
      }
    }
  }
  return newProcess;
}

/***/ },

/***/ "./src/deepCyclicCopy.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = deepCyclicCopy;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const EMPTY = new Set();
function deepCyclicCopy(value, options, cycles = new WeakMap()) {
  options = {
    blacklist: EMPTY,
    keepPrototype: false,
    ...options
  };
  if (typeof value !== 'object' || value === null || Buffer.isBuffer(value)) {
    return value;
  } else if (cycles.has(value)) {
    return cycles.get(value);
  } else if (Array.isArray(value)) {
    return deepCyclicCopyArray(value, options, cycles);
  } else {
    return deepCyclicCopyObject(value, options, cycles);
  }
}
function deepCyclicCopyObject(object, options, cycles) {
  const newObject = options.keepPrototype ? Object.create(Object.getPrototypeOf(object)) : {};
  const descriptors = Object.getOwnPropertyDescriptors(object);
  cycles.set(object, newObject);
  for (const key of Object.keys(descriptors)) {
    if (options.blacklist && options.blacklist.has(key)) {
      delete descriptors[key];
      continue;
    }
    const descriptor = descriptors[key];
    if (descriptor.value !== undefined) {
      descriptor.value = deepCyclicCopy(descriptor.value, {
        blacklist: EMPTY,
        keepPrototype: options.keepPrototype
      }, cycles);
    }
    descriptor.configurable = true;
  }
  return Object.defineProperties(newObject, descriptors);
}
function deepCyclicCopyArray(array, options, cycles) {
  const newArray = options.keepPrototype ? new (Object.getPrototypeOf(array).constructor)(array.length) : [];
  const length = array.length;
  cycles.set(array, newArray);
  for (let i = 0; i < length; i++) {
    newArray[i] = deepCyclicCopy(array[i], {
      blacklist: EMPTY,
      keepPrototype: options.keepPrototype
    }, cycles);
  }
  return newArray;
}

/***/ },

/***/ "./src/formatTime.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = formatTime;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function formatTime(time, prefixPower = -3, padLeftLength = 0) {
  const prefixes = ['n', 'μ', 'm', ''];
  const prefixIndex = Math.max(0, Math.min(Math.trunc(prefixPower / 3) + prefixes.length - 1, prefixes.length - 1));
  return `${String(time).padStart(padLeftLength)} ${prefixes[prefixIndex]}s`;
}

/***/ },

/***/ "./src/garbage-collection-utils.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.canDeleteProperties = canDeleteProperties;
exports.deleteProperties = deleteProperties;
exports.initializeGarbageCollectionUtils = initializeGarbageCollectionUtils;
exports.protectProperties = protectProperties;
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

/**
 * The symbol that is set on the global object to store the deletion mode.
 */
const DELETION_MODE_SYMBOL = Symbol.for('$$jest-deletion-mode');

/**
 * The symbol that is set on objects to protect them from deletion.
 *
 * If the value is an empty array, then all properties will be protected.
 * If the value is an array of strings or symbols, then only those properties will be protected.
 */
const PROTECT_SYMBOL = Symbol.for('$$jest-protect-from-deletion');

/**
 *  - <b>off</b>: deletion is completely turned off.
 *  - <b>soft</b>: doesn't delete objects, but instead wraps their getter/setter with a deprecation warning.
 *  - <b>on</b>: actually delete objects (using `delete`).
 */

/**
 * Initializes the garbage collection utils with the given deletion mode.
 *
 * @param globalObject the global object on which to store the deletion mode.
 * @param deletionMode the deletion mode to use.
 */
function initializeGarbageCollectionUtils(globalObject, deletionMode) {
  const currentMode = Reflect.get(globalObject, DELETION_MODE_SYMBOL);
  if (currentMode && currentMode !== deletionMode) {
    console.warn(_chalk().default.yellow(['[jest-util] garbage collection deletion mode already initialized, ignoring new mode', `  Current: '${currentMode}'`, `  Given: '${deletionMode}'`].join('\n')));
    return;
  }
  Reflect.set(globalObject, DELETION_MODE_SYMBOL, deletionMode);
}

/**
 * Deletes all the properties from the given value (if it's an object),
 * unless the value was protected via {@link #protectProperties}.
 *
 * @param value the given value.
 */
function deleteProperties(value) {
  if (getDeletionMode() !== 'off' && canDeleteProperties(value)) {
    const protectedKeys = getProtectedKeys(value, Reflect.get(value, PROTECT_SYMBOL));
    for (const key of Reflect.ownKeys(value)) {
      if (!protectedKeys.includes(key) && key !== PROTECT_SYMBOL) {
        deleteProperty(value, key);
      }
    }
  }
}

/**
 * Protects the given value from being deleted by {@link #deleteProperties}.
 *
 * @param value The given value.
 * @param properties If the array contains any property,
 * then only these properties will be protected; otherwise if the array is empty,
 * all properties will be protected.
 * @param depth Determines how "deep" the protection should be.
 * A value of 0 means that only the top-most properties will be protected,
 * while a value larger than 0 means that deeper levels of nesting will be protected as well.
 */
function protectProperties(value, properties = [], depth = 2) {
  if (getDeletionMode() === 'off') {
    return false;
  }

  // Reflect.get may cause deprecation warnings, so we disable them temporarily
  const originalEmitWarning = process.emitWarning;
  try {
    // eslint-disable-next-line @typescript-eslint/no-empty-function
    process.emitWarning = () => {};
    if (depth >= 0 && canDeleteProperties(value) && !Reflect.has(value, PROTECT_SYMBOL)) {
      const result = Reflect.defineProperty(value, PROTECT_SYMBOL, {
        configurable: true,
        enumerable: false,
        value: properties,
        writable: true
      });
      for (const key of getProtectedKeys(value, properties)) {
        try {
          const nested = Reflect.get(value, key);
          protectProperties(nested, [], depth - 1);
        } catch {
          // Reflect.get might fail in certain edge-cases
          // Instead of failing the entire process, we will skip the property.
        }
      }
      return result;
    }
    return false;
  } finally {
    process.emitWarning = originalEmitWarning;
  }
}

/**
 * Whether the given value has properties that can be deleted (regardless of protection).
 *
 * @param value The given value.
 */
function canDeleteProperties(value) {
  if (value !== null) {
    const type = typeof value;
    return type === 'object' || type === 'function';
  }
  return false;
}

/**
 * Deletes the property of the given key from the given object.
 *
 * @param obj the given object.
 * @param key the given key.
 * @param mode there are two possible modes of deletion:
 *  - <b>soft</b>: doesn't delete the object, but instead wraps its getter/setter with a deprecation warning.
 *  - <b>hard</b>: actually deletes the object (`delete`).
 *
 * @returns whether the deletion was successful or not.
 */
function deleteProperty(obj, key) {
  const descriptor = Reflect.getOwnPropertyDescriptor(obj, key);
  if (!descriptor?.configurable) {
    return false;
  }
  if (getDeletionMode() === 'on') {
    return Reflect.deleteProperty(obj, key);
  }
  const originalGetter = descriptor.get ?? (() => descriptor.value);
  const originalSetter = descriptor.set ?? (value => Reflect.set(obj, key, value));
  return Reflect.defineProperty(obj, key, {
    configurable: true,
    enumerable: descriptor.enumerable,
    get() {
      emitAccessWarning(obj, key);
      return originalGetter();
    },
    set(value) {
      emitAccessWarning(obj, key);
      return originalSetter(value);
    }
  });
}
function getDeletionMode() {
  return Reflect.get(globalThis, DELETION_MODE_SYMBOL) ?? 'off';
}
const warningCache = new WeakSet();
function emitAccessWarning(obj, key) {
  if (warningCache.has(obj)) {
    return;
  }
  const objName = obj?.constructor?.name ?? 'unknown';
  const propertyName = typeof key === 'symbol' ? key.description : key;
  process.emitWarning(`'${propertyName}' property was accessed on [${objName}] after it was soft deleted`, {
    code: 'JEST-01',
    detail: ['Jest deletes objects that were set on the global scope between test files to reduce memory leaks.', 'Currently it only "soft" deletes them and emits this warning if those objects were accessed after their deletion.', 'In future versions of Jest, this behavior will change to "on", which will likely fail tests.', 'You can change the behavior in your test configuration now to reduce memory usage.'].map(s => `  ${s}`).join('\n'),
    type: 'DeprecationWarning'
  });
  warningCache.add(obj);
}
function getProtectedKeys(value, properties) {
  if (properties === undefined) {
    return [];
  }
  const protectedKeys = properties.length > 0 ? properties : Reflect.ownKeys(value);
  return protectedKeys.filter(key => PROTECT_SYMBOL !== key);
}

/***/ },

/***/ "./src/globsToMatcher.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = globsToMatcher;
function _picomatch() {
  const data = _interopRequireDefault(require("picomatch"));
  _picomatch = function () {
    return data;
  };
  return data;
}
var _replacePathSepForGlob = _interopRequireDefault(__webpack_require__("./src/replacePathSepForGlob.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const globsToMatchersMap = new Map();

/**
 * Converts a list of globs into a function that matches a path against the
 * globs.
 *
 * Every time picomatch is called, it will parse the glob strings and turn
 * them into regexp instances. Instead of calling picomatch repeatedly with
 * the same globs, we can use this function which will build the picomatch
 * matchers ahead of time and then have an optimized path for determining
 * whether an individual path matches.
 *
 * This function is intended to match the behavior of `micromatch()`.
 *
 * @example
 * const isMatch = globsToMatcher(['*.js', '!*.test.js']);
 * isMatch('pizza.js'); // true
 * isMatch('pizza.test.js'); // false
 */
function globsToMatcher(globs, picomatchOptions) {
  const dot = picomatchOptions?.dot ?? true;
  if (globs.length === 0) {
    // Since there were no globs given, we can simply have a fast path here and
    // return with a very simple function.
    return () => false;
  }
  const matchers = globs.map(glob => {
    if (!globsToMatchersMap.has(glob)) {
      const isMatch = (0, _picomatch().default)(glob, {
        dot,
        ...picomatchOptions
      }, true);
      const matcher = {
        isMatch,
        // Matchers that are negated have different behavior than matchers that
        // are not negated, so we need to store this information ahead of time.
        negated: isMatch.state.negated || !!isMatch.state.negatedExtglob
      };
      globsToMatchersMap.set(glob, matcher);
    }
    return globsToMatchersMap.get(glob);
  });
  return path => {
    const replacedPath = (0, _replacePathSepForGlob.default)(path);
    let kept = undefined;
    let negatives = 0;
    for (const matcher of matchers) {
      const {
        isMatch,
        negated
      } = matcher;
      if (negated) {
        negatives++;
      }
      const matched = isMatch(replacedPath);
      if (!matched && negated) {
        // The path was not matched, and the matcher is a negated matcher, so we
        // want to omit the path. This means that the negative matcher is
        // filtering the path out.
        kept = false;
      } else if (matched && !negated) {
        // The path was matched, and the matcher is not a negated matcher, so we
        // want to keep the path.
        kept = true;
      }
    }

    // If all of the globs were negative globs, then we want to include the path
    // as long as it was not explicitly not kept. Otherwise only include
    // the path if it was kept. This allows sets of globs that are all negated
    // to allow some paths to be matched, while sets of globs that are mixed
    // negated and non-negated to cause the negated matchers to only omit paths
    // and not keep them.
    return negatives === matchers.length ? kept !== false : !!kept;
  };
}

/***/ },

/***/ "./src/installCommonGlobals.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = installCommonGlobals;
function fs() {
  const data = _interopRequireWildcard(require("graceful-fs"));
  fs = function () {
    return data;
  };
  return data;
}
var _createProcessObject = _interopRequireDefault(__webpack_require__("./src/createProcessObject.ts"));
var _deepCyclicCopy = _interopRequireDefault(__webpack_require__("./src/deepCyclicCopy.ts"));
var _garbageCollectionUtils = __webpack_require__("./src/garbage-collection-utils.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const DTRACE = Object.keys(globalThis).filter(key => key.startsWith('DTRACE'));
function installCommonGlobals(globalObject, globals, garbageCollectionDeletionMode) {
  globalObject.process = (0, _createProcessObject.default)();
  const symbol = globalObject.Symbol;
  // Keep a reference to some globals that Jest needs
  Object.defineProperties(globalObject, {
    [symbol.for('jest-native-promise')]: {
      enumerable: false,
      value: Promise,
      writable: false
    },
    [symbol.for('jest-native-now')]: {
      enumerable: false,
      value: globalObject.Date.now.bind(globalObject.Date),
      writable: false
    },
    [symbol.for('jest-native-read-file')]: {
      enumerable: false,
      value: fs().readFileSync.bind(fs()),
      writable: false
    },
    [symbol.for('jest-native-write-file')]: {
      enumerable: false,
      value: fs().writeFileSync.bind(fs()),
      writable: false
    },
    [symbol.for('jest-native-exists-file')]: {
      enumerable: false,
      value: fs().existsSync.bind(fs()),
      writable: false
    },
    'jest-symbol-do-not-touch': {
      enumerable: false,
      value: symbol,
      writable: false
    }
  });

  // Forward some APIs.
  for (const dtrace of DTRACE) {
    // @ts-expect-error: no index
    globalObject[dtrace] = function (...args) {
      // @ts-expect-error: no index
      return globalThis[dtrace].apply(this, args);
    };
  }
  if (garbageCollectionDeletionMode) {
    (0, _garbageCollectionUtils.initializeGarbageCollectionUtils)(globalObject, garbageCollectionDeletionMode);
  }
  return Object.assign(globalObject, (0, _deepCyclicCopy.default)(globals));
}

/***/ },

/***/ "./src/interopRequireDefault.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = interopRequireDefault;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// copied from https://github.com/babel/babel/blob/56044c7851d583d498f919e9546caddf8f80a72f/packages/babel-helpers/src/helpers.js#L558-L562
// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
function interopRequireDefault(obj) {
  return obj && obj.__esModule ? obj : {
    default: obj
  };
}

/***/ },

/***/ "./src/invariant.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = invariant;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function invariant(condition, message = '') {
  if (!condition) {
    throw new Error(message);
  }
}

/***/ },

/***/ "./src/isError.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.isError = void 0;
function _types() {
  const data = require("node:util/types");
  _types = function () {
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

const isError = exports.isError = typeof Error.isError === 'function' ? Error.isError : _types().isNativeError;

/***/ },

/***/ "./src/isInteractive.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _ciInfo() {
  const data = require("ci-info");
  _ciInfo = function () {
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

function checkIsInteractive() {
  if (_ciInfo().isCI) {
    return false;
  }

  // this can happen in a browser with polyfills: https://github.com/defunctzombie/node-process/issues/41
  if (process.stdout == null) {
    return false;
  }
  if (process.stdout.isTTY) {
    return process.env.TERM !== 'dumb';
  }
  return false;
}
const isInteractive = checkIsInteractive();
var _default = exports["default"] = isInteractive;

/***/ },

/***/ "./src/isNonNullable.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = isNonNullable;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function isNonNullable(value) {
  return value != null;
}

/***/ },

/***/ "./src/isPromise.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = isPromise;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function isPromise(candidate) {
  return candidate != null && (typeof candidate === 'object' || typeof candidate === 'function') && typeof candidate.then === 'function';
}

/***/ },

/***/ "./src/pluralize.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = pluralize;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function pluralize(word, count, ending = 's') {
  return `${count} ${word}${count === 1 ? '' : ending}`;
}

/***/ },

/***/ "./src/preRunMessage.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.print = print;
exports.remove = remove;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
var _clearLine = _interopRequireDefault(__webpack_require__("./src/clearLine.ts"));
var _isInteractive = _interopRequireDefault(__webpack_require__("./src/isInteractive.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function print(stream) {
  if (_isInteractive.default) {
    stream.write(_chalk().default.bold.dim('Determining test suites to run...'));
  }
}
function remove(stream) {
  if (_isInteractive.default) {
    (0, _clearLine.default)(stream);
  }
}

/***/ },

/***/ "./src/replacePathSepForGlob.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = replacePathSepForGlob;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function replacePathSepForGlob(path) {
  return path.replaceAll(/\\(?![$()+.?^{}])/g, '/');
}

/***/ },

/***/ "./src/requireOrImportModule.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = requireOrImportModule;
function _nodePath() {
  const data = require("node:path");
  _nodePath = function () {
    return data;
  };
  return data;
}
function _nodeUrl() {
  const data = require("node:url");
  _nodeUrl = function () {
    return data;
  };
  return data;
}
var _interopRequireDefault = _interopRequireDefault2(__webpack_require__("./src/interopRequireDefault.ts"));
function _interopRequireDefault2(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

async function importModule(filePath, applyInteropRequireDefault) {
  try {
    const moduleUrl = (0, _nodeUrl().pathToFileURL)(filePath);

    // node `import()` supports URL, but TypeScript doesn't know that
    const importedModule = await import(/* webpackIgnore: true */moduleUrl.href);
    if (!applyInteropRequireDefault) {
      return importedModule;
    }
    if (!importedModule.default) {
      throw new Error(`Jest: Failed to load ESM at ${filePath} - did you use a default export?`);
    }
    return importedModule.default;
  } catch (error) {
    if (error.message === 'Not supported') {
      throw new Error(`Jest: Your version of Node does not support dynamic import - please enable it or use a .cjs file extension for file ${filePath}`);
    }
    throw error;
  }
}
async function requireOrImportModule(filePath, applyInteropRequireDefault = true) {
  if (!(0, _nodePath().isAbsolute)(filePath) && filePath[0] === '.') {
    throw new Error(`Jest: requireOrImportModule path must be absolute, was "${filePath}"`);
  }
  try {
    if (filePath.endsWith('.mjs') || filePath.endsWith('.mts')) {
      return importModule(filePath, applyInteropRequireDefault);
    }
    const requiredModule = require(filePath);
    if (!applyInteropRequireDefault) {
      return requiredModule;
    }
    return (0, _interopRequireDefault.default)(requiredModule).default;
  } catch (error) {
    if (error.code === 'ERR_REQUIRE_ESM' || error.code === 'ERR_REQUIRE_ASYNC_MODULE') {
      return importModule(filePath, applyInteropRequireDefault);
    } else {
      throw error;
    }
  }
}

/***/ },

/***/ "./src/setGlobal.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = setGlobal;
var _garbageCollectionUtils = __webpack_require__("./src/garbage-collection-utils.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function setGlobal(globalToMutate, key, value, afterTeardown = 'clean') {
  Reflect.set(globalToMutate, key, value);
  if (afterTeardown === 'retain' && (0, _garbageCollectionUtils.canDeleteProperties)(value)) {
    (0, _garbageCollectionUtils.protectProperties)(value);
  }
}

/***/ },

/***/ "./src/specialChars.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.ICONS = exports.CLEAR = exports.ARROW = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const isWindows = process.platform === 'win32';
const ARROW = exports.ARROW = ' \u203A ';
const ICONS = exports.ICONS = {
  failed: isWindows ? '\u00D7' : '\u2715',
  pending: '\u25CB',
  success: isWindows ? '\u221A' : '\u2713',
  todo: '\u270E'
};
const CLEAR = exports.CLEAR = isWindows ? '\u001B[2J\u001B[0f' : '\u001B[2J\u001B[3J\u001B[H';

/***/ },

/***/ "./src/tryRealpath.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = tryRealpath;
function _gracefulFs() {
  const data = require("graceful-fs");
  _gracefulFs = function () {
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

function tryRealpath(path) {
  try {
    path = _gracefulFs().realpathSync.native(path);
  } catch (error) {
    if (error.code !== 'ENOENT' && error.code !== 'EISDIR') {
      throw error;
    }
  }
  return path;
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
Object.defineProperty(exports, "ErrorWithStack", ({
  enumerable: true,
  get: function () {
    return _ErrorWithStack.default;
  }
}));
Object.defineProperty(exports, "canDeleteProperties", ({
  enumerable: true,
  get: function () {
    return _garbageCollectionUtils.canDeleteProperties;
  }
}));
Object.defineProperty(exports, "clearLine", ({
  enumerable: true,
  get: function () {
    return _clearLine.default;
  }
}));
Object.defineProperty(exports, "convertDescriptorToString", ({
  enumerable: true,
  get: function () {
    return _convertDescriptorToString.default;
  }
}));
Object.defineProperty(exports, "createDirectory", ({
  enumerable: true,
  get: function () {
    return _createDirectory.default;
  }
}));
Object.defineProperty(exports, "deepCyclicCopy", ({
  enumerable: true,
  get: function () {
    return _deepCyclicCopy.default;
  }
}));
Object.defineProperty(exports, "deleteProperties", ({
  enumerable: true,
  get: function () {
    return _garbageCollectionUtils.deleteProperties;
  }
}));
Object.defineProperty(exports, "formatTime", ({
  enumerable: true,
  get: function () {
    return _formatTime.default;
  }
}));
Object.defineProperty(exports, "globsToMatcher", ({
  enumerable: true,
  get: function () {
    return _globsToMatcher.default;
  }
}));
Object.defineProperty(exports, "initializeGarbageCollectionUtils", ({
  enumerable: true,
  get: function () {
    return _garbageCollectionUtils.initializeGarbageCollectionUtils;
  }
}));
Object.defineProperty(exports, "installCommonGlobals", ({
  enumerable: true,
  get: function () {
    return _installCommonGlobals.default;
  }
}));
Object.defineProperty(exports, "interopRequireDefault", ({
  enumerable: true,
  get: function () {
    return _interopRequireDefault.default;
  }
}));
Object.defineProperty(exports, "invariant", ({
  enumerable: true,
  get: function () {
    return _invariant.default;
  }
}));
Object.defineProperty(exports, "isError", ({
  enumerable: true,
  get: function () {
    return _isError.isError;
  }
}));
Object.defineProperty(exports, "isInteractive", ({
  enumerable: true,
  get: function () {
    return _isInteractive.default;
  }
}));
Object.defineProperty(exports, "isNonNullable", ({
  enumerable: true,
  get: function () {
    return _isNonNullable.default;
  }
}));
Object.defineProperty(exports, "isPromise", ({
  enumerable: true,
  get: function () {
    return _isPromise.default;
  }
}));
Object.defineProperty(exports, "pluralize", ({
  enumerable: true,
  get: function () {
    return _pluralize.default;
  }
}));
exports.preRunMessage = void 0;
Object.defineProperty(exports, "protectProperties", ({
  enumerable: true,
  get: function () {
    return _garbageCollectionUtils.protectProperties;
  }
}));
Object.defineProperty(exports, "replacePathSepForGlob", ({
  enumerable: true,
  get: function () {
    return _replacePathSepForGlob.default;
  }
}));
Object.defineProperty(exports, "requireOrImportModule", ({
  enumerable: true,
  get: function () {
    return _requireOrImportModule.default;
  }
}));
Object.defineProperty(exports, "setGlobal", ({
  enumerable: true,
  get: function () {
    return _setGlobal.default;
  }
}));
exports.specialChars = void 0;
Object.defineProperty(exports, "tryRealpath", ({
  enumerable: true,
  get: function () {
    return _tryRealpath.default;
  }
}));
var preRunMessage = _interopRequireWildcard(__webpack_require__("./src/preRunMessage.ts"));
exports.preRunMessage = preRunMessage;
var specialChars = _interopRequireWildcard(__webpack_require__("./src/specialChars.ts"));
exports.specialChars = specialChars;
var _clearLine = _interopRequireDefault2(__webpack_require__("./src/clearLine.ts"));
var _createDirectory = _interopRequireDefault2(__webpack_require__("./src/createDirectory.ts"));
var _ErrorWithStack = _interopRequireDefault2(__webpack_require__("./src/ErrorWithStack.ts"));
var _installCommonGlobals = _interopRequireDefault2(__webpack_require__("./src/installCommonGlobals.ts"));
var _interopRequireDefault = _interopRequireDefault2(__webpack_require__("./src/interopRequireDefault.ts"));
var _isInteractive = _interopRequireDefault2(__webpack_require__("./src/isInteractive.ts"));
var _isPromise = _interopRequireDefault2(__webpack_require__("./src/isPromise.ts"));
var _setGlobal = _interopRequireDefault2(__webpack_require__("./src/setGlobal.ts"));
var _deepCyclicCopy = _interopRequireDefault2(__webpack_require__("./src/deepCyclicCopy.ts"));
var _convertDescriptorToString = _interopRequireDefault2(__webpack_require__("./src/convertDescriptorToString.ts"));
var _replacePathSepForGlob = _interopRequireDefault2(__webpack_require__("./src/replacePathSepForGlob.ts"));
var _globsToMatcher = _interopRequireDefault2(__webpack_require__("./src/globsToMatcher.ts"));
var _pluralize = _interopRequireDefault2(__webpack_require__("./src/pluralize.ts"));
var _formatTime = _interopRequireDefault2(__webpack_require__("./src/formatTime.ts"));
var _tryRealpath = _interopRequireDefault2(__webpack_require__("./src/tryRealpath.ts"));
var _requireOrImportModule = _interopRequireDefault2(__webpack_require__("./src/requireOrImportModule.ts"));
var _invariant = _interopRequireDefault2(__webpack_require__("./src/invariant.ts"));
var _isNonNullable = _interopRequireDefault2(__webpack_require__("./src/isNonNullable.ts"));
var _garbageCollectionUtils = __webpack_require__("./src/garbage-collection-utils.ts");
var _isError = __webpack_require__("./src/isError.ts");
function _interopRequireDefault2(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
})();

module.exports = __webpack_exports__;
/******/ })()
;