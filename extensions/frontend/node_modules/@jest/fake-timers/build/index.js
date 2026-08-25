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

/***/ "./src/legacyFakeTimers.ts"
(__unused_webpack_module, exports) {



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
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/* eslint-disable local/prefer-spread-eventually */

const MS_IN_A_YEAR = 31_536_000_000;
class FakeTimers {
  _cancelledTicks;
  _config;
  _disposed;
  _fakeTimerAPIs;
  _fakingTime = false;
  _global;
  _immediates;
  _maxLoops;
  _moduleMocker;
  _now;
  _ticks;
  _timerAPIs;
  _timers;
  _uuidCounter;
  _timerConfig;
  constructor({
    global,
    moduleMocker,
    timerConfig,
    config,
    maxLoops
  }) {
    this._global = global;
    this._timerConfig = timerConfig;
    this._config = config;
    this._maxLoops = maxLoops || 100_000;
    this._uuidCounter = 1;
    this._moduleMocker = moduleMocker;

    // Store original timer APIs for future reference
    this._timerAPIs = {
      cancelAnimationFrame: global.cancelAnimationFrame,
      clearImmediate: global.clearImmediate,
      clearInterval: global.clearInterval,
      clearTimeout: global.clearTimeout,
      nextTick: global.process && global.process.nextTick,
      requestAnimationFrame: global.requestAnimationFrame,
      setImmediate: global.setImmediate,
      setInterval: global.setInterval,
      setTimeout: global.setTimeout
    };
    this._disposed = false;
    this.reset();
  }
  clearAllTimers() {
    this._immediates = [];
    this._timers.clear();
  }
  dispose() {
    this._disposed = true;
    this.clearAllTimers();
  }
  reset() {
    this._cancelledTicks = {};
    this._now = 0;
    this._ticks = [];
    this._immediates = [];
    this._timers = new Map();
  }
  now() {
    if (this._fakingTime) {
      return this._now;
    }
    return Date.now();
  }
  runAllTicks() {
    this._checkFakeTimers();
    // Only run a generous number of ticks and then bail.
    // This is just to help avoid recursive loops
    let i;
    for (i = 0; i < this._maxLoops; i++) {
      const tick = this._ticks.shift();
      if (tick === undefined) {
        break;
      }
      if (!Object.prototype.hasOwnProperty.call(this._cancelledTicks, tick.uuid)) {
        // Callback may throw, so update the map prior calling.
        this._cancelledTicks[tick.uuid] = true;
        tick.callback();
      }
    }
    if (i === this._maxLoops) {
      throw new Error(`Ran ${this._maxLoops} ticks, and there are still more! ` + "Assuming we've hit an infinite recursion and bailing out...");
    }
  }
  runAllImmediates() {
    this._checkFakeTimers();
    // Only run a generous number of immediates and then bail.
    let i;
    for (i = 0; i < this._maxLoops; i++) {
      const immediate = this._immediates.shift();
      if (immediate === undefined) {
        break;
      }
      this._runImmediate(immediate);
    }
    if (i === this._maxLoops) {
      throw new Error(`Ran ${this._maxLoops} immediates, and there are still more! Assuming ` + "we've hit an infinite recursion and bailing out...");
    }
  }
  _runImmediate(immediate) {
    try {
      immediate.callback();
    } finally {
      this._fakeClearImmediate(immediate.uuid);
    }
  }
  runAllTimers() {
    this._checkFakeTimers();
    this.runAllTicks();
    this.runAllImmediates();

    // Only run a generous number of timers and then bail.
    // This is just to help avoid recursive loops
    let i;
    for (i = 0; i < this._maxLoops; i++) {
      const nextTimerHandleAndExpiry = this._getNextTimerHandleAndExpiry();

      // If there are no more timer handles, stop!
      if (nextTimerHandleAndExpiry === null) {
        break;
      }
      const [nextTimerHandle, expiry] = nextTimerHandleAndExpiry;
      this._now = expiry;
      this._runTimerHandle(nextTimerHandle);

      // Some of the immediate calls could be enqueued
      // during the previous handling of the timers, we should
      // run them as well.
      if (this._immediates.length > 0) {
        this.runAllImmediates();
      }
      if (this._ticks.length > 0) {
        this.runAllTicks();
      }
    }
    if (i === this._maxLoops) {
      throw new Error(`Ran ${this._maxLoops} timers, and there are still more! ` + "Assuming we've hit an infinite recursion and bailing out...");
    }
  }
  runOnlyPendingTimers() {
    // We need to hold the current shape of `this._timers` because existing
    // timers can add new ones to the map and hence would run more than necessary.
    // See https://github.com/jestjs/jest/pull/4608 for details
    const timerEntries = [...this._timers.entries()];
    this._checkFakeTimers();
    for (const _immediate of this._immediates) this._runImmediate(_immediate);
    for (const [timerHandle, timer] of timerEntries.sort(([, left], [, right]) => left.expiry - right.expiry)) {
      this._now = timer.expiry;
      this._runTimerHandle(timerHandle);
    }
  }
  advanceTimersToNextTimer(steps = 1) {
    if (steps < 1) {
      return;
    }
    const nextExpiry = [...this._timers.values()].reduce((minExpiry, timer) => {
      if (minExpiry === null || timer.expiry < minExpiry) return timer.expiry;
      return minExpiry;
    }, null);
    if (nextExpiry !== null) {
      this.advanceTimersByTime(nextExpiry - this._now);
      this.advanceTimersToNextTimer(steps - 1);
    }
  }
  advanceTimersByTime(msToRun) {
    this._checkFakeTimers();
    let msRemaining = typeof msToRun === 'number' ? msToRun : msToRun.total({
      unit: 'millisecond'
    });
    // Only run a generous number of timers and then bail.
    // This is just to help avoid recursive loops
    let i;
    for (i = 0; i < this._maxLoops; i++) {
      const timerHandleAndExpiry = this._getNextTimerHandleAndExpiry();

      // If there are no more timer handles, stop!
      if (timerHandleAndExpiry === null) {
        break;
      }
      const [timerHandle, nextTimerExpiry] = timerHandleAndExpiry;
      if (this._now + msRemaining < nextTimerExpiry) {
        // There are no timers between now and the target we're running to
        break;
      } else {
        msRemaining -= nextTimerExpiry - this._now;
        this._now = nextTimerExpiry;
        this._runTimerHandle(timerHandle);
      }
    }

    // Advance the clock by whatever time we still have left to run
    this._now += msRemaining;
    if (i === this._maxLoops) {
      throw new Error(`Ran ${this._maxLoops} timers, and there are still more! ` + "Assuming we've hit an infinite recursion and bailing out...");
    }
  }
  runWithRealTimers(cb) {
    const prevClearImmediate = this._global.clearImmediate;
    const prevClearInterval = this._global.clearInterval;
    const prevClearTimeout = this._global.clearTimeout;
    const prevNextTick = this._global.process.nextTick;
    const prevSetImmediate = this._global.setImmediate;
    const prevSetInterval = this._global.setInterval;
    const prevSetTimeout = this._global.setTimeout;
    this.useRealTimers();
    let cbErr = null;
    let errThrown = false;
    try {
      cb();
    } catch (error) {
      errThrown = true;
      cbErr = error;
    }
    this._global.clearImmediate = prevClearImmediate;
    this._global.clearInterval = prevClearInterval;
    this._global.clearTimeout = prevClearTimeout;
    this._global.process.nextTick = prevNextTick;
    this._global.setImmediate = prevSetImmediate;
    this._global.setInterval = prevSetInterval;
    this._global.setTimeout = prevSetTimeout;
    if (errThrown) {
      throw cbErr;
    }
  }
  useRealTimers() {
    const global = this._global;
    if (typeof global.cancelAnimationFrame === 'function') {
      (0, _jestUtil().setGlobal)(global, 'cancelAnimationFrame', this._timerAPIs.cancelAnimationFrame);
    }
    if (typeof global.clearImmediate === 'function') {
      (0, _jestUtil().setGlobal)(global, 'clearImmediate', this._timerAPIs.clearImmediate);
    }
    (0, _jestUtil().setGlobal)(global, 'clearInterval', this._timerAPIs.clearInterval);
    (0, _jestUtil().setGlobal)(global, 'clearTimeout', this._timerAPIs.clearTimeout);
    if (typeof global.requestAnimationFrame === 'function') {
      (0, _jestUtil().setGlobal)(global, 'requestAnimationFrame', this._timerAPIs.requestAnimationFrame);
    }
    if (typeof global.setImmediate === 'function') {
      (0, _jestUtil().setGlobal)(global, 'setImmediate', this._timerAPIs.setImmediate);
    }
    (0, _jestUtil().setGlobal)(global, 'setInterval', this._timerAPIs.setInterval);
    (0, _jestUtil().setGlobal)(global, 'setTimeout', this._timerAPIs.setTimeout);
    global.process.nextTick = this._timerAPIs.nextTick;
    this._fakingTime = false;
  }
  useFakeTimers() {
    this._createMocks();
    const global = this._global;
    if (typeof global.cancelAnimationFrame === 'function') {
      (0, _jestUtil().setGlobal)(global, 'cancelAnimationFrame', this._fakeTimerAPIs.cancelAnimationFrame);
    }
    if (typeof global.clearImmediate === 'function') {
      (0, _jestUtil().setGlobal)(global, 'clearImmediate', this._fakeTimerAPIs.clearImmediate);
    }
    (0, _jestUtil().setGlobal)(global, 'clearInterval', this._fakeTimerAPIs.clearInterval);
    (0, _jestUtil().setGlobal)(global, 'clearTimeout', this._fakeTimerAPIs.clearTimeout);
    if (typeof global.requestAnimationFrame === 'function') {
      (0, _jestUtil().setGlobal)(global, 'requestAnimationFrame', this._fakeTimerAPIs.requestAnimationFrame);
    }
    if (typeof global.setImmediate === 'function') {
      (0, _jestUtil().setGlobal)(global, 'setImmediate', this._fakeTimerAPIs.setImmediate);
    }
    (0, _jestUtil().setGlobal)(global, 'setInterval', this._fakeTimerAPIs.setInterval);
    (0, _jestUtil().setGlobal)(global, 'setTimeout', this._fakeTimerAPIs.setTimeout);
    global.process.nextTick = this._fakeTimerAPIs.nextTick;
    this._fakingTime = true;
  }
  getTimerCount() {
    this._checkFakeTimers();
    return this._timers.size + this._immediates.length + this._ticks.length;
  }
  _checkFakeTimers() {
    if (!this._fakingTime) {
      this._global.console.warn('A function to advance timers was called but the timers APIs are not mocked ' + 'with fake timers. Call `jest.useFakeTimers({legacyFakeTimers: true})` ' + 'in this test file or enable fake timers for all tests by setting ' + "{'enableGlobally': true, 'legacyFakeTimers': true} in " + `Jest configuration file.\nStack Trace:\n${(0, _jestMessageUtil().formatStackTrace)(
      // eslint-disable-next-line unicorn/error-message
      new Error().stack, this._config, {
        noStackTrace: false
      })}`);
    }
  }
  #createMockFunction(implementation) {
    return this._moduleMocker.fn(implementation.bind(this));
  }
  _createMocks() {
    const promisifiableFakeSetTimeout = this.#createMockFunction(this._fakeSetTimeout);
    // @ts-expect-error: no index
    promisifiableFakeSetTimeout[_nodeUtil().promisify.custom] = (delay, arg) => new Promise(resolve => promisifiableFakeSetTimeout(resolve, delay, arg));
    this._fakeTimerAPIs = {
      cancelAnimationFrame: this.#createMockFunction(this._fakeClearTimer),
      clearImmediate: this.#createMockFunction(this._fakeClearImmediate),
      clearInterval: this.#createMockFunction(this._fakeClearTimer),
      clearTimeout: this.#createMockFunction(this._fakeClearTimer),
      nextTick: this.#createMockFunction(this._fakeNextTick),
      requestAnimationFrame: this.#createMockFunction(this._fakeRequestAnimationFrame),
      setImmediate: this.#createMockFunction(this._fakeSetImmediate),
      setInterval: this.#createMockFunction(this._fakeSetInterval),
      setTimeout: promisifiableFakeSetTimeout
    };
  }
  _fakeClearTimer(timerRef) {
    const uuid = this._timerConfig.refToId(timerRef);
    if (uuid) {
      this._timers.delete(String(uuid));
    }
  }
  _fakeClearImmediate(uuid) {
    this._immediates = this._immediates.filter(immediate => immediate.uuid !== uuid);
  }
  _fakeNextTick(callback, ...args) {
    if (this._disposed) {
      return;
    }
    const uuid = String(this._uuidCounter++);
    this._ticks.push({
      callback: () => callback.apply(null, args),
      uuid
    });
    const cancelledTicks = this._cancelledTicks;
    this._timerAPIs.nextTick(() => {
      if (!Object.prototype.hasOwnProperty.call(cancelledTicks, uuid)) {
        // Callback may throw, so update the map prior calling.
        cancelledTicks[uuid] = true;
        callback.apply(null, args);
      }
    });
  }
  _fakeRequestAnimationFrame(callback) {
    return this._fakeSetTimeout(() => {
      // TODO: Use performance.now() once it's mocked
      callback(this._now);
    }, 1000 / 60);
  }
  _fakeSetImmediate(callback, ...args) {
    if (this._disposed) {
      return null;
    }
    const uuid = String(this._uuidCounter++);
    this._immediates.push({
      callback: () => callback.apply(null, args),
      uuid
    });
    this._timerAPIs.setImmediate(() => {
      if (!this._disposed) {
        if (this._immediates.some(x => x.uuid === uuid)) {
          try {
            callback.apply(null, args);
          } finally {
            this._fakeClearImmediate(uuid);
          }
        }
      }
    });
    return uuid;
  }
  _fakeSetInterval(callback, intervalDelay, ...args) {
    if (this._disposed) {
      return null;
    }
    if (intervalDelay == null) {
      intervalDelay = 0;
    }
    const uuid = this._uuidCounter++;
    this._timers.set(String(uuid), {
      callback: () => callback.apply(null, args),
      expiry: this._now + intervalDelay,
      interval: intervalDelay,
      type: 'interval'
    });
    return this._timerConfig.idToRef(uuid);
  }
  _fakeSetTimeout(callback, delay, ...args) {
    if (this._disposed) {
      return null;
    }

    // eslint-disable-next-line no-bitwise,unicorn/prefer-math-trunc
    delay = Number(delay) | 0;
    const uuid = this._uuidCounter++;
    this._timers.set(String(uuid), {
      callback: () => callback.apply(null, args),
      expiry: this._now + delay,
      interval: undefined,
      type: 'timeout'
    });
    return this._timerConfig.idToRef(uuid);
  }
  _getNextTimerHandleAndExpiry() {
    let nextTimerHandle = null;
    let soonestTime = MS_IN_A_YEAR;
    for (const [uuid, timer] of this._timers.entries()) {
      if (timer.expiry < soonestTime) {
        soonestTime = timer.expiry;
        nextTimerHandle = uuid;
      }
    }
    if (nextTimerHandle === null) {
      return null;
    }
    return [nextTimerHandle, soonestTime];
  }
  _runTimerHandle(timerHandle) {
    const timer = this._timers.get(timerHandle);
    if (!timer) {
      // Timer has been cleared - we'll hit this when a timer is cleared within
      // another timer in runOnlyPendingTimers
      return;
    }
    switch (timer.type) {
      case 'timeout':
        this._timers.delete(timerHandle);
        timer.callback();
        break;
      case 'interval':
        timer.expiry = this._now + (timer.interval || 0);
        timer.callback();
        break;
      default:
        throw new Error(`Unexpected timer type: ${timer.type}`);
    }
  }
}
exports["default"] = FakeTimers;

/***/ },

/***/ "./src/modernFakeTimers.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _fakeTimers() {
  const data = require("@sinonjs/fake-timers");
  _fakeTimers = function () {
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
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class FakeTimers {
  _clock;
  _config;
  _fakingTime;
  _global;
  _fakeTimers;
  constructor({
    global,
    config
  }) {
    this._global = global;
    this._config = config;
    this._fakingTime = false;
    this._fakeTimers = (0, _fakeTimers().withGlobal)(global);
  }
  clearAllTimers() {
    if (this._fakingTime) {
      this._clock.reset();
    }
  }
  dispose() {
    this.useRealTimers();
  }
  runAllTimers() {
    if (this._checkFakeTimers()) {
      this._clock.runAll();
    }
  }
  async runAllTimersAsync() {
    if (this._checkFakeTimers()) {
      await this._clock.runAllAsync();
    }
  }
  runOnlyPendingTimers() {
    if (this._checkFakeTimers()) {
      this._clock.runToLast();
    }
  }
  async runOnlyPendingTimersAsync() {
    if (this._checkFakeTimers()) {
      await this._clock.runToLastAsync();
    }
  }
  advanceTimersToNextTimer(steps = 1) {
    if (this._checkFakeTimers()) {
      for (let i = steps; i > 0; i--) {
        this._clock.next();
        // Fire all timers at this point: https://github.com/sinonjs/fake-timers/issues/250
        this._clock.tick(0);
        if (this._clock.countTimers() === 0) {
          break;
        }
      }
    }
  }
  async advanceTimersToNextTimerAsync(steps = 1) {
    if (this._checkFakeTimers()) {
      for (let i = steps; i > 0; i--) {
        await this._clock.nextAsync();
        // Fire all timers at this point: https://github.com/sinonjs/fake-timers/issues/250
        await this._clock.tickAsync(0);
        if (this._clock.countTimers() === 0) {
          break;
        }
      }
    }
  }
  advanceTimersByTime(msToRun) {
    if (this._checkFakeTimers()) {
      // TODO: pass msToRun directly once https://github.com/sinonjs/fake-timers/pull/574 is published
      this._clock.tick(typeof msToRun === 'number' ? msToRun : msToRun.total({
        unit: 'millisecond'
      }));
    }
  }
  async advanceTimersByTimeAsync(msToRun) {
    if (this._checkFakeTimers()) {
      // TODO: pass msToRun directly once https://github.com/sinonjs/fake-timers/pull/574 is published
      await this._clock.tickAsync(typeof msToRun === 'number' ? msToRun : msToRun.total({
        unit: 'millisecond'
      }));
    }
  }
  advanceTimersToNextFrame() {
    if (this._checkFakeTimers()) {
      this._clock.runToFrame();
    }
  }
  runAllTicks() {
    if (this._checkFakeTimers()) {
      this._clock.runMicrotasks();
    }
  }
  useRealTimers() {
    if (this._fakingTime) {
      this._clock.uninstall();
      this._fakingTime = false;
    }
  }
  useFakeTimers(fakeTimersConfig) {
    if (this._fakingTime) {
      this._clock.uninstall();
    }
    this._clock = this._fakeTimers.install(this._toSinonFakeTimersConfig(fakeTimersConfig));
    this._fakingTime = true;
  }
  reset() {
    if (this._checkFakeTimers()) {
      const {
        now
      } = this._clock;
      this._clock.reset();
      this._clock.setSystemTime(now);
    }
  }
  setSystemTime(now) {
    if (this._checkFakeTimers()) {
      this._clock.setSystemTime(now);
    }
  }
  setTimerTickMode(tickModeConfig) {
    if (this._checkFakeTimers()) {
      this._clock.setTickMode(tickModeConfig);
    }
  }
  getRealSystemTime() {
    return Date.now();
  }
  now() {
    if (this._fakingTime) {
      return this._clock.now;
    }
    return Date.now();
  }
  getTimerCount() {
    if (this._checkFakeTimers()) {
      return this._clock.countTimers();
    }
    return 0;
  }
  _checkFakeTimers() {
    if (!this._fakingTime) {
      this._global.console.warn('A function to advance timers was called but the timers APIs are not replaced ' + 'with fake timers. Call `jest.useFakeTimers()` in this test file or enable ' + "fake timers for all tests by setting 'fakeTimers': {'enableGlobally': true} " + `in Jest configuration file.\nStack Trace:\n${(0, _jestMessageUtil().formatStackTrace)(
      // eslint-disable-next-line unicorn/error-message
      new Error().stack, this._config, {
        noStackTrace: false
      })}`);
    }
    return this._fakingTime;
  }
  _toSinonFakeTimersConfig(fakeTimersConfig = {}) {
    fakeTimersConfig = {
      ...this._config.fakeTimers,
      ...fakeTimersConfig
    };
    const advanceTimeDelta = typeof fakeTimersConfig.advanceTimers === 'number' ? fakeTimersConfig.advanceTimers : undefined;
    const toFake = new Set(Object.keys(this._fakeTimers.timers));
    if (fakeTimersConfig.doNotFake) for (const nameOfFakeableAPI of fakeTimersConfig.doNotFake) {
      toFake.delete(nameOfFakeableAPI);
    }
    return {
      advanceTimeDelta,
      loopLimit: fakeTimersConfig.timerLimit || 100_000,
      now: fakeTimersConfig.now ?? Date.now(),
      shouldAdvanceTime: Boolean(fakeTimersConfig.advanceTimers),
      shouldClearNativeTimers: true,
      toFake: [...toFake]
    };
  }
}
exports["default"] = FakeTimers;

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
Object.defineProperty(exports, "LegacyFakeTimers", ({
  enumerable: true,
  get: function () {
    return _legacyFakeTimers.default;
  }
}));
Object.defineProperty(exports, "ModernFakeTimers", ({
  enumerable: true,
  get: function () {
    return _modernFakeTimers.default;
  }
}));
var _legacyFakeTimers = _interopRequireDefault(__webpack_require__("./src/legacyFakeTimers.ts"));
var _modernFakeTimers = _interopRequireDefault(__webpack_require__("./src/modernFakeTimers.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
})();

module.exports = __webpack_exports__;
/******/ })()
;