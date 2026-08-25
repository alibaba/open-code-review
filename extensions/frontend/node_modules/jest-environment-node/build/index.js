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
var __webpack_exports__ = {};
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = exports.TestEnvironment = void 0;
function _nodeVm() {
  const data = require("node:vm");
  _nodeVm = function () {
    return data;
  };
  return data;
}
function _fakeTimers() {
  const data = require("@jest/fake-timers");
  _fakeTimers = function () {
    return data;
  };
  return data;
}
function _jestMock() {
  const data = require("jest-mock");
  _jestMock = function () {
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
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// some globals we do not want, either because deprecated or we set it ourselves
const denyList = new Set(['GLOBAL', 'root', 'global', 'globalThis', 'Buffer', 'ArrayBuffer', 'Uint8Array',
// if env is loaded within a jest test
'jest-symbol-do-not-touch']);
// Storage proxies (Node 25+) emit a warning on any Reflect.get access when
// --localstorage-file is not set. We install them as non-caching passthrough
// getters so they never land in GlobalProxy.propertyToValue and are never
// inspected by deleteProperties at teardown.
const storageGlobals = new Set(['localStorage', 'sessionStorage']);
const nodeGlobals = new Map(Object.getOwnPropertyNames(globalThis).filter(global => !denyList.has(global)).map(nodeGlobalsKey => {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis, nodeGlobalsKey);
  if (!descriptor) {
    throw new Error(`No property descriptor for ${nodeGlobalsKey}, this is a bug in Jest.`);
  }
  return [nodeGlobalsKey, descriptor];
}));
function isString(value) {
  return typeof value === 'string';
}
const timerIdToRef = id => ({
  id,
  ref() {
    return this;
  },
  unref() {
    return this;
  }
});
const timerRefToId = timer => timer?.id;
class NodeEnvironment {
  context;
  fakeTimers;
  fakeTimersModern;
  global;
  moduleMocker;
  customExportConditions = ['node', 'node-addons'];
  _configuredExportConditions;
  _globalProxy;

  // while `context` is unused, it should always be passed
  constructor(config, _context) {
    const {
      projectConfig
    } = config;
    const globalsCleanupMode = readGlobalsCleanupConfig(projectConfig);
    (0, _jestUtil().initializeGarbageCollectionUtils)(globalThis, globalsCleanupMode);
    this._globalProxy = new GlobalProxy();
    this.context = (0, _nodeVm().createContext)(this._globalProxy.proxy());
    const global = (0, _nodeVm().runInContext)('this', Object.assign(this.context, projectConfig.testEnvironmentOptions));
    this.global = global;
    const contextGlobals = new Set(Object.getOwnPropertyNames(global));
    for (const [nodeGlobalsKey, descriptor] of nodeGlobals) {
      if (!storageGlobals.has(nodeGlobalsKey)) {
        (0, _jestUtil().protectProperties)(globalThis[nodeGlobalsKey]);
      }
      if (!contextGlobals.has(nodeGlobalsKey)) {
        if (storageGlobals.has(nodeGlobalsKey)) {
          Object.defineProperty(global, nodeGlobalsKey, {
            configurable: true,
            enumerable: descriptor.enumerable,
            get: () => globalThis[nodeGlobalsKey],
            set(value) {
              Object.defineProperty(global, nodeGlobalsKey, {
                configurable: true,
                enumerable: descriptor.enumerable,
                value,
                writable: true
              });
            }
          });
        } else if (descriptor.configurable) {
          Object.defineProperty(global, nodeGlobalsKey, {
            configurable: true,
            enumerable: descriptor.enumerable,
            get() {
              const value = globalThis[nodeGlobalsKey];

              // override lazy getter
              Object.defineProperty(global, nodeGlobalsKey, {
                configurable: true,
                enumerable: descriptor.enumerable,
                value,
                writable: true
              });
              return value;
            },
            set(value) {
              // override lazy getter
              Object.defineProperty(global, nodeGlobalsKey, {
                configurable: true,
                enumerable: descriptor.enumerable,
                value,
                writable: true
              });
            }
          });
        } else if ('value' in descriptor) {
          Object.defineProperty(global, nodeGlobalsKey, {
            configurable: false,
            enumerable: descriptor.enumerable,
            value: descriptor.value,
            writable: descriptor.writable
          });
        } else {
          Object.defineProperty(global, nodeGlobalsKey, {
            configurable: false,
            enumerable: descriptor.enumerable,
            get: descriptor.get,
            set: descriptor.set
          });
        }
      }
    }
    global.global = global;
    global.Buffer = Buffer;
    global.ArrayBuffer = ArrayBuffer;
    // TextEncoder (global or via 'util') references a Uint8Array constructor
    // different than the global one used by users in tests. This makes sure the
    // same constructor is referenced by both.
    global.Uint8Array = Uint8Array;
    (0, _jestUtil().installCommonGlobals)(global, projectConfig.globals, globalsCleanupMode);
    if ('asyncDispose' in Symbol && !('asyncDispose' in global.Symbol)) {
      const globalSymbol = global.Symbol;
      // @ts-expect-error - it's readonly - but we have checked above that it's not there
      globalSymbol.asyncDispose = globalSymbol.for('nodejs.asyncDispose');
      // @ts-expect-error - it's readonly - but we have checked above that it's not there
      globalSymbol.dispose = globalSymbol.for('nodejs.dispose');
    }

    // Node's error-message stack size is limited at 10, but it's pretty useful
    // to see more than that when a test fails.
    global.Error.stackTraceLimit = 100;
    if ('customExportConditions' in projectConfig.testEnvironmentOptions) {
      const {
        customExportConditions
      } = projectConfig.testEnvironmentOptions;
      if (Array.isArray(customExportConditions) && customExportConditions.every(isString)) {
        this._configuredExportConditions = customExportConditions;
      } else {
        throw new Error('Custom export conditions specified but they are not an array of strings');
      }
    }
    this.moduleMocker = new (_jestMock().ModuleMocker)(global);
    this.fakeTimers = new (_fakeTimers().LegacyFakeTimers)({
      config: projectConfig,
      global,
      moduleMocker: this.moduleMocker,
      timerConfig: {
        idToRef: timerIdToRef,
        refToId: timerRefToId
      }
    });
    this.fakeTimersModern = new (_fakeTimers().ModernFakeTimers)({
      config: projectConfig,
      global
    });
    this._globalProxy.envSetupCompleted();
  }

  // eslint-disable-next-line @typescript-eslint/no-empty-function
  async setup() {}
  async teardown() {
    if (this.fakeTimers) {
      this.fakeTimers.dispose();
    }
    if (this.fakeTimersModern) {
      this.fakeTimersModern.dispose();
    }
    this.context = null;
    this.fakeTimers = null;
    this.fakeTimersModern = null;
    this._globalProxy.clear();
  }
  exportConditions() {
    return this._configuredExportConditions ?? this.customExportConditions;
  }
  getVmContext() {
    return this.context;
  }
}
exports["default"] = NodeEnvironment;
const TestEnvironment = exports.TestEnvironment = NodeEnvironment;

/**
 * Creates a new empty global object and wraps it with a {@link Proxy}.
 *
 * The purpose is to register any property set on the global object,
 * and {@link #deleteProperties} on them at environment teardown,
 * to clean up memory and prevent leaks.
 */
class GlobalProxy {
  global = Object.create(Object.getPrototypeOf(globalThis));
  globalProxy = new Proxy(this.global, this);
  isEnvSetup = false;
  propertyToValue = new Map();
  leftovers = [];
  constructor() {
    this.register = this.register.bind(this);
  }
  proxy() {
    return this.globalProxy;
  }

  /**
   * Marks that the environment setup has completed, and properties set on
   * the global object from now on should be deleted at teardown.
   */
  envSetupCompleted() {
    this.isEnvSetup = true;
  }

  /**
   * Deletes any property that was set on the global object, except for:
   * 1. Properties that were set before {@link #envSetupCompleted} was invoked.
   * 2. Properties protected by {@link #protectProperties}.
   */
  clear() {
    for (const {
      value
    } of [...[...this.propertyToValue.entries()].map(([property, value]) => ({
      property,
      value
    })), ...this.leftovers]) {
      (0, _jestUtil().deleteProperties)(value);
    }
    this.propertyToValue.clear();
    this.leftovers = [];
    this.global = {};
    this.globalProxy = {};
  }
  defineProperty(target, property, attributes) {
    const newAttributes = {
      ...attributes
    };
    if ('set' in newAttributes && newAttributes.set !== undefined) {
      const originalSet = newAttributes.set;
      const register = this.register;
      newAttributes.set = value => {
        originalSet(value);
        const newValue = Reflect.get(target, property);
        register(property, newValue);
      };
    }
    const result = Reflect.defineProperty(target, property, newAttributes);
    if ('value' in newAttributes) {
      this.register(property, newAttributes.value);
    }
    return result;
  }
  deleteProperty(target, property) {
    const result = Reflect.deleteProperty(target, property);
    const value = this.propertyToValue.get(property);
    if (value) {
      this.leftovers.push({
        property,
        value
      });
      this.propertyToValue.delete(property);
    }
    return result;
  }
  register(property, value) {
    const currentValue = this.propertyToValue.get(property);
    if (value !== currentValue) {
      if (!this.isEnvSetup && (0, _jestUtil().canDeleteProperties)(value)) {
        (0, _jestUtil().protectProperties)(value);
      }
      if (currentValue) {
        this.leftovers.push({
          property,
          value: currentValue
        });
      }
      this.propertyToValue.set(property, value);
    }
  }
}
function readGlobalsCleanupConfig(projectConfig) {
  const rawConfig = projectConfig.testEnvironmentOptions.globalsCleanup;
  const config = rawConfig?.toString()?.toLowerCase();
  switch (config) {
    case 'off':
    case 'on':
    case 'soft':
      return config;
    default:
      {
        if (config !== undefined) {
          (0, _jestValidate().logValidationWarning)('testEnvironmentOptions.globalsCleanup', `Unknown value given: ${rawConfig}`, 'Available options are: [on, soft, off]');
        }
        return 'soft';
      }
  }
}
})();

module.exports = __webpack_exports__;
/******/ })()
;