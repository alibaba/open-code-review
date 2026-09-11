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

/***/ "./src/Farm.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _FifoQueue = _interopRequireDefault(__webpack_require__("./src/FifoQueue.ts"));
var _types = __webpack_require__("./src/types.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class Farm {
  _computeWorkerKey;
  _workerSchedulingPolicy;
  _cacheKeys = Object.create(null);
  _locks = [];
  _offset = 0;
  _taskQueue;
  constructor(_numOfWorkers, _callback, options = {}) {
    this._numOfWorkers = _numOfWorkers;
    this._callback = _callback;
    this._computeWorkerKey = options.computeWorkerKey;
    this._workerSchedulingPolicy = options.workerSchedulingPolicy ?? 'round-robin';
    this._taskQueue = options.taskQueue ?? new _FifoQueue.default();
  }
  doWork(method, ...args) {
    const customMessageListeners = new Set();
    const addCustomMessageListener = listener => {
      customMessageListeners.add(listener);
      return () => {
        customMessageListeners.delete(listener);
      };
    };
    const onCustomMessage = message => {
      for (const listener of customMessageListeners) listener(message);
    };
    const promise = new Promise(
    // Bind args to this function so it won't reference to the parent scope.
    // This prevents a memory leak in v8, because otherwise the function will
    // retain args for the closure.
    ((args, resolve, reject) => {
      const computeWorkerKey = this._computeWorkerKey;
      const request = [_types.CHILD_MESSAGE_CALL, false, method, args];
      let worker = null;
      let hash = null;
      if (computeWorkerKey) {
        hash = computeWorkerKey.call(this, method, ...args);
        worker = hash == null ? null : this._cacheKeys[hash];
      }
      const onStart = worker => {
        if (hash != null) {
          this._cacheKeys[hash] = worker;
        }
      };
      const onEnd = (error, result) => {
        customMessageListeners.clear();
        if (error) {
          reject(error);
        } else {
          resolve(result);
        }
      };
      const task = {
        onCustomMessage,
        onEnd,
        onStart,
        request
      };
      if (worker) {
        this._taskQueue.enqueue(task, worker.getWorkerId());
        this._process(worker.getWorkerId());
      } else {
        this._push(task);
      }
    }).bind(null, args));
    promise.UNSTABLE_onCustomMessage = addCustomMessageListener;
    return promise;
  }
  _process(workerId) {
    if (this._isLocked(workerId)) {
      return this;
    }
    const task = this._taskQueue.dequeue(workerId);
    if (!task) {
      return this;
    }
    if (task.request[1]) {
      throw new Error('Queue implementation returned processed task');
    }

    // Reference the task object outside so it won't be retained by onEnd,
    // and other properties of the task object, such as task.request can be
    // garbage collected.
    let taskOnEnd = task.onEnd;
    const onEnd = (error, result) => {
      if (taskOnEnd) {
        taskOnEnd(error, result);
      }
      taskOnEnd = null;
      this._unlock(workerId);
      this._process(workerId);
    };
    task.request[1] = true;
    this._lock(workerId);
    this._callback(workerId, task.request, task.onStart, onEnd, task.onCustomMessage);
    return this;
  }
  _push(task) {
    this._taskQueue.enqueue(task);
    const offset = this._getNextWorkerOffset();
    for (let i = 0; i < this._numOfWorkers; i++) {
      this._process((offset + i) % this._numOfWorkers);
      if (task.request[1]) {
        break;
      }
    }
    return this;
  }
  _getNextWorkerOffset() {
    switch (this._workerSchedulingPolicy) {
      case 'in-order':
        return 0;
      case 'round-robin':
        return this._offset++;
    }
  }
  _lock(workerId) {
    this._locks[workerId] = true;
  }
  _unlock(workerId) {
    this._locks[workerId] = false;
  }
  _isLocked(workerId) {
    return this._locks[workerId];
  }
}
exports["default"] = Farm;

/***/ },

/***/ "./src/FifoQueue.ts"
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

/**
 * First-in, First-out task queue that manages a dedicated pool
 * for each worker as well as a shared queue. The FIFO ordering is guaranteed
 * across the worker specific and shared queue.
 */
class FifoQueue {
  _workerQueues = [];
  _sharedQueue = new InternalQueue();
  enqueue(task, workerId) {
    if (workerId == null) {
      this._sharedQueue.enqueue(task);
      return;
    }
    let workerQueue = this._workerQueues[workerId];
    if (workerQueue == null) {
      workerQueue = this._workerQueues[workerId] = new InternalQueue();
    }
    const sharedTop = this._sharedQueue.peekLast();
    const item = {
      previousSharedTask: sharedTop,
      task
    };
    workerQueue.enqueue(item);
  }
  dequeue(workerId) {
    const workerTop = this._workerQueues[workerId]?.peek();
    const sharedTaskIsProcessed = workerTop?.previousSharedTask?.request[1] ?? true;

    // Process the top task from the shared queue if
    // - there's no task in the worker specific queue or
    // - if the non-worker-specific task after which this worker specific task
    //   has been queued wasn't processed yet
    if (workerTop != null && sharedTaskIsProcessed) {
      return this._workerQueues[workerId]?.dequeue()?.task ?? null;
    }
    return this._sharedQueue.dequeue();
  }
}
exports["default"] = FifoQueue;
/**
 * FIFO queue for a single worker / shared queue.
 */
class InternalQueue {
  _head = null;
  _last = null;
  enqueue(value) {
    const item = {
      next: null,
      value
    };
    if (this._last == null) {
      this._head = item;
    } else {
      this._last.next = item;
    }
    this._last = item;
  }
  dequeue() {
    if (this._head == null) {
      return null;
    }
    const item = this._head;
    this._head = item.next;
    if (this._head == null) {
      this._last = null;
    }
    return item.value;
  }
  peek() {
    return this._head?.value ?? null;
  }
  peekLast() {
    return this._last?.value ?? null;
  }
}

/***/ },

/***/ "./src/PriorityQueue.ts"
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

/**
 * Priority queue that processes tasks in natural ordering (lower priority first)
 * according to the priority computed by the function passed in the constructor.
 *
 * FIFO ordering isn't guaranteed for tasks with the same priority.
 *
 * Worker specific tasks with the same priority as a non-worker specific task
 * are always processed first.
 */
class PriorityQueue {
  _queue = [];
  _sharedQueue = new MinHeap();
  constructor(_computePriority) {
    this._computePriority = _computePriority;
  }
  enqueue(task, workerId) {
    if (workerId == null) {
      this._enqueue(task, this._sharedQueue);
    } else {
      const queue = this._getWorkerQueue(workerId);
      this._enqueue(task, queue);
    }
  }
  _enqueue(task, queue) {
    const item = {
      priority: this._computePriority(task.request[2], ...task.request[3]),
      task
    };
    queue.add(item);
  }
  dequeue(workerId) {
    const workerQueue = this._getWorkerQueue(workerId);
    const workerTop = workerQueue.peek();
    const sharedTop = this._sharedQueue.peek();

    // use the task from the worker queue if there's no task in the shared queue
    // or if the priority of the worker queue is smaller or equal to the
    // priority of the top task in the shared queue. The tasks of the
    // worker specific queue are preferred because no other worker can pick this
    // specific task up.
    if (sharedTop == null || workerTop != null && workerTop.priority <= sharedTop.priority) {
      return workerQueue.poll()?.task ?? null;
    }
    return this._sharedQueue.poll().task;
  }
  _getWorkerQueue(workerId) {
    let queue = this._queue[workerId];
    if (queue == null) {
      queue = this._queue[workerId] = new MinHeap();
    }
    return queue;
  }
}
exports["default"] = PriorityQueue;
class MinHeap {
  _heap = [];
  peek() {
    return this._heap[0] ?? null;
  }
  add(item) {
    const nodes = this._heap;
    nodes.push(item);
    if (nodes.length === 1) {
      return;
    }
    let currentIndex = nodes.length - 1;

    // Bubble up the added node as long as the parent is bigger
    while (currentIndex > 0) {
      const parentIndex = Math.floor((currentIndex + 1) / 2) - 1;
      const parent = nodes[parentIndex];
      if (parent.priority <= item.priority) {
        break;
      }
      nodes[currentIndex] = parent;
      nodes[parentIndex] = item;
      currentIndex = parentIndex;
    }
  }
  poll() {
    const nodes = this._heap;
    const result = nodes[0];
    const lastElement = nodes.pop();

    // heap was empty or removed the last element
    if (result == null || nodes.length === 0) {
      return result ?? null;
    }
    let index = 0;
    nodes[0] = lastElement ?? null;
    const element = nodes[0];
    while (true) {
      let swapIndex = null;
      const rightChildIndex = (index + 1) * 2;
      const leftChildIndex = rightChildIndex - 1;
      const rightChild = nodes[rightChildIndex];
      const leftChild = nodes[leftChildIndex];

      // if the left child is smaller, swap with the left
      if (leftChild != null && leftChild.priority < element.priority) {
        swapIndex = leftChildIndex;
      }

      // If the right child is smaller or the right child is smaller than the left
      // then swap with the right child
      if (rightChild != null && rightChild.priority < (swapIndex == null ? element : leftChild).priority) {
        swapIndex = rightChildIndex;
      }
      if (swapIndex == null) {
        break;
      }
      nodes[index] = nodes[swapIndex];
      nodes[swapIndex] = element;
      index = swapIndex;
    }
    return result;
  }
}

/***/ },

/***/ "./src/WorkerPool.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _BaseWorkerPool = _interopRequireDefault(__webpack_require__("./src/base/BaseWorkerPool.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class WorkerPool extends _BaseWorkerPool.default {
  send(workerId, request, onStart, onEnd, onCustomMessage) {
    this.restartWorkerIfShutDown(workerId);
    this.getWorkerById(workerId).send(request, onStart, onEnd, onCustomMessage);
  }
  createWorker(workerOptions) {
    let Worker;
    if (this._options.enableWorkerThreads) {
      Worker = (__webpack_require__("./src/workers/NodeThreadsWorker.ts")/* ["default"] */ .A);
    } else {
      Worker = (__webpack_require__("./src/workers/ChildProcessWorker.ts")/* ["default"] */ .Ay);
    }
    return new Worker(workerOptions);
  }
}
var _default = exports["default"] = WorkerPool;

/***/ },

/***/ "./src/base/BaseWorkerPool.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _mergeStream() {
  const data = _interopRequireDefault(require("merge-stream"));
  _mergeStream = function () {
    return data;
  };
  return data;
}
var _types = __webpack_require__("./src/types.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

/* istanbul ignore next */
// eslint-disable-next-line @typescript-eslint/no-empty-function
const emptyMethod = () => {};
class BaseWorkerPool {
  _stderr;
  _stdout;
  _options;
  _workers;
  _workerPath;
  constructor(workerPath, options) {
    this._options = options;
    this._workerPath = workerPath;
    this._workers = Array.from({
      length: options.numWorkers
    });
    const stdout = (0, _mergeStream().default)();
    const stderr = (0, _mergeStream().default)();
    const {
      forkOptions,
      maxRetries,
      resourceLimits,
      setupArgs
    } = options;
    for (let i = 0; i < options.numWorkers; i++) {
      const workerOptions = {
        forkOptions,
        idleMemoryLimit: this._options.idleMemoryLimit,
        maxRetries,
        resourceLimits,
        setupArgs,
        workerId: i,
        workerPath
      };
      const worker = this.createWorker(workerOptions);
      const workerStdout = worker.getStdout();
      const workerStderr = worker.getStderr();
      if (workerStdout) {
        stdout.add(workerStdout);
      }
      if (workerStderr) {
        stderr.add(workerStderr);
      }
      this._workers[i] = worker;
    }
    this._stdout = stdout;
    this._stderr = stderr;
  }
  getStderr() {
    return this._stderr;
  }
  getStdout() {
    return this._stdout;
  }
  getWorkers() {
    return this._workers;
  }
  getWorkerById(workerId) {
    return this._workers[workerId];
  }
  restartWorkerIfShutDown(workerId) {
    if (this._workers[workerId].state === _types.WorkerStates.SHUT_DOWN) {
      const {
        forkOptions,
        maxRetries,
        resourceLimits,
        setupArgs
      } = this._options;
      const workerOptions = {
        forkOptions,
        idleMemoryLimit: this._options.idleMemoryLimit,
        maxRetries,
        resourceLimits,
        setupArgs,
        workerId,
        workerPath: this._workerPath
      };
      const worker = this.createWorker(workerOptions);
      this._workers[workerId] = worker;
    }
  }
  createWorker(_workerOptions) {
    throw new Error('Missing method createWorker in WorkerPool');
  }
  async start() {
    await Promise.all(this._workers.map(async worker => {
      await worker.waitForWorkerReady();
      await new Promise((resolve, reject) => {
        worker.send([_types.CHILD_MESSAGE_CALL_SETUP], emptyMethod, error => {
          if (error) {
            reject(error);
          } else {
            resolve();
          }
        }, emptyMethod);
      });
    }));
  }
  async end() {
    // We do not cache the request object here. If so, it would only be only
    // processed by one of the workers, and we want them all to close.
    const workerExitPromises = this._workers.map(async worker => {
      worker.send([_types.CHILD_MESSAGE_END, false], emptyMethod, emptyMethod, emptyMethod);

      // Schedule a force exit in case worker fails to exit gracefully so
      // await worker.waitForExit() never takes longer than FORCE_EXIT_DELAY
      let forceExited = false;
      const forceExitTimeout = setTimeout(() => {
        worker.forceExit();
        forceExited = true;
      }, this._options.workerGracefulExitTimeout ?? 500);
      await worker.waitForExit();
      // Worker ideally exited gracefully, don't send force exit then
      clearTimeout(forceExitTimeout);
      return forceExited;
    });
    const workerExits = await Promise.all(workerExitPromises);
    return workerExits.reduce((result, forceExited) => ({
      forceExited: result.forceExited || forceExited
    }), {
      forceExited: false
    });
  }
}
exports["default"] = BaseWorkerPool;

/***/ },

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

/***/ "./src/workers/ChildProcessWorker.ts"
(__unused_webpack_module, exports, __webpack_require__) {

var __webpack_unused_export__;


__webpack_unused_export__ = ({
  value: true
});
exports.Ay = __webpack_unused_export__ = void 0;
function _nodeChild_process() {
  const data = require("node:child_process");
  _nodeChild_process = function () {
    return data;
  };
  return data;
}
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
    return data;
  };
  return data;
}
function _mergeStream() {
  const data = _interopRequireDefault(require("merge-stream"));
  _mergeStream = function () {
    return data;
  };
  return data;
}
function _supportsColor() {
  const data = require("supports-color");
  _supportsColor = function () {
    return data;
  };
  return data;
}
var _types = __webpack_require__("./src/types.ts");
var _WorkerAbstract = _interopRequireDefault(__webpack_require__("./src/workers/WorkerAbstract.ts"));
var _safeMessageTransferring = __webpack_require__("./src/workers/safeMessageTransferring.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const SIGNAL_BASE_EXIT_CODE = 128;
const SIGKILL_EXIT_CODE = SIGNAL_BASE_EXIT_CODE + 9;
const SIGTERM_EXIT_CODE = SIGNAL_BASE_EXIT_CODE + 15;

// How long to wait after SIGTERM before sending SIGKILL
const SIGKILL_DELAY = __webpack_unused_export__ = 500;

/**
 * This class wraps the child process and provides a nice interface to
 * communicate with. It takes care of:
 *
 *  - Re-spawning the process if it dies.
 *  - Queues calls while the worker is busy.
 *  - Re-sends the requests if the worker blew up.
 *
 * The reason for queueing them here (since childProcess.send also has an
 * internal queue) is because the worker could be doing asynchronous work, and
 * this would lead to the child process to read its receiving buffer and start a
 * second call. By queueing calls here, we don't send the next call to the
 * children until we receive the result of the previous one.
 *
 * As soon as a request starts to be processed by a worker, its "processed"
 * field is changed to "true", so that other workers which might encounter the
 * same call skip it.
 */
class ChildProcessWorker extends _WorkerAbstract.default {
  _child;
  _options;
  _request;
  _retries;
  _onProcessEnd;
  _onCustomMessage;
  _stdout;
  _stderr;
  _stderrBuffer = [];
  _memoryUsagePromise;
  _resolveMemoryUsage;
  _childIdleMemoryUsage;
  _childIdleMemoryUsageLimit;
  _memoryUsageCheck = false;
  _childWorkerPath;
  constructor(options) {
    super(options);
    this._options = options;
    this._request = null;
    this._stdout = null;
    this._stderr = null;
    this._childIdleMemoryUsage = null;
    this._childIdleMemoryUsageLimit = options.idleMemoryLimit ?? null;
    this._childWorkerPath = options.childWorkerPath || require.resolve('./processChild');
    this.state = _types.WorkerStates.STARTING;
    this.initialize();
  }
  initialize() {
    if (this.state === _types.WorkerStates.OUT_OF_MEMORY || this.state === _types.WorkerStates.SHUTTING_DOWN || this.state === _types.WorkerStates.SHUT_DOWN) {
      return;
    }
    if (this._child && this._child.connected) {
      this._child.kill('SIGKILL');
    }
    this.state = _types.WorkerStates.STARTING;
    const forceColor = _supportsColor().stdout ? {
      FORCE_COLOR: '1'
    } : {};
    const silent = this._options.silent ?? true;
    if (!silent) {
      // NOTE: Detecting an out of memory crash is independent of idle memory usage monitoring. We want to
      // monitor for a crash occurring so that it can be handled as required and so we can tell the difference
      // between an OOM crash and another kind of crash. We need to do this because if a worker crashes due to
      // an OOM event sometimes it isn't seen by the worker pool and it just sits there waiting for the worker
      // to respond and it never will.
      console.warn('Unable to detect out of memory event if silent === false');
    }
    this._stderrBuffer = [];
    const options = {
      cwd: process.cwd(),
      env: {
        ...process.env,
        JEST_WORKER_ID: String(this._options.workerId + 1),
        // 0-indexed workerId, 1-indexed JEST_WORKER_ID
        ...forceColor
      },
      // Suppress --debug / --inspect flags while preserving others (like --harmony).
      execArgv: process.execArgv.filter(v => !/^--(debug|inspect)/.test(v)),
      // default to advanced serialization in order to match worker threads
      serialization: 'advanced',
      silent,
      ...this._options.forkOptions
    };
    this._child = (0, _nodeChild_process().fork)(this._childWorkerPath, [], options);
    if (this._child.stdout) {
      if (!this._stdout) {
        // We need to add a permanent stream to the merged stream to prevent it
        // from ending when the subprocess stream ends
        this._stdout = (0, _mergeStream().default)(this._getFakeStream());
      }
      this._stdout.add(this._child.stdout);
    }
    if (this._child.stderr) {
      if (!this._stderr) {
        // We need to add a permanent stream to the merged stream to prevent it
        // from ending when the subprocess stream ends
        this._stderr = (0, _mergeStream().default)(this._getFakeStream());
      }
      this._stderr.add(this._child.stderr);
      this._child.stderr.on('data', this.stderrDataHandler.bind(this));
    }
    this._child.on('message', this._onMessage.bind(this));
    this._child.on('exit', this._onExit.bind(this));
    this._child.on('disconnect', this._onDisconnect.bind(this));
    this._child.send([_types.CHILD_MESSAGE_INITIALIZE, false, this._options.workerPath, this._options.setupArgs]);
    this._retries++;

    // If we exceeded the amount of retries, we will emulate an error reply
    // coming from the child. This avoids code duplication related with cleaning
    // the queue, and scheduling the next call.
    if (this._retries > this._options.maxRetries) {
      const error = new Error(`Jest worker encountered ${this._retries} child process exceptions, exceeding retry limit`);
      this._onMessage([_types.PARENT_MESSAGE_CLIENT_ERROR, error.name, error.message, error.stack, {
        type: 'WorkerError'
      }]);

      // Clear the request so we don't keep executing it.
      this._request = null;
    }
    this.state = _types.WorkerStates.OK;
    if (this._resolveWorkerReady) {
      this._resolveWorkerReady();
    }
  }
  stderrDataHandler(chunk) {
    if (chunk) {
      this._stderrBuffer.push(Buffer.from(chunk));
    }
    this._detectOutOfMemoryCrash();
    if (this.state === _types.WorkerStates.OUT_OF_MEMORY) {
      this._workerReadyPromise = undefined;
      this._resolveWorkerReady = undefined;
      this.killChild();
      this._shutdown();
    }
  }
  _detectOutOfMemoryCrash() {
    try {
      const bufferStr = Buffer.concat(this._stderrBuffer).toString('utf8');
      if (bufferStr.includes('heap out of memory') || bufferStr.includes('allocation failure;') || bufferStr.includes('Last few GCs')) {
        if (this.state === _types.WorkerStates.OK || this.state === _types.WorkerStates.STARTING || this.state === _types.WorkerStates.SHUT_DOWN) {
          this.state = _types.WorkerStates.OUT_OF_MEMORY;
        }
      }
    } catch (error) {
      console.error('Error looking for out of memory crash', error);
    }
  }
  _onDisconnect() {
    this._workerReadyPromise = undefined;
    this._resolveWorkerReady = undefined;
    this._detectOutOfMemoryCrash();
    if (this.state === _types.WorkerStates.OUT_OF_MEMORY) {
      this.killChild();
      this._shutdown();
    }
  }
  _onMessage(response) {
    // Ignore messages not intended for us
    if (!Array.isArray(response)) return;

    // TODO: Add appropriate type check
    let error;
    switch (response[0]) {
      case _types.PARENT_MESSAGE_OK:
        this._onProcessEnd(null, (0, _safeMessageTransferring.unpackMessage)(response[1]));
        break;
      case _types.PARENT_MESSAGE_CLIENT_ERROR:
        error = response[4];
        if (error != null && typeof error === 'object') {
          const extra = error;
          // @ts-expect-error: no index
          const NativeCtor = globalThis[response[1]];
          const Ctor = typeof NativeCtor === 'function' ? NativeCtor : Error;
          error = new Ctor(response[2]);
          error.type = response[1];
          error.stack = response[3];
          for (const key in extra) {
            error[key] = extra[key];
          }
        }
        this._onProcessEnd(error, null);
        break;
      case _types.PARENT_MESSAGE_SETUP_ERROR:
        error = new Error(`Error when calling setup: ${response[2]}`);
        error.type = response[1];
        error.stack = response[3];
        this._onProcessEnd(error, null);
        break;
      case _types.PARENT_MESSAGE_CUSTOM:
        this._onCustomMessage((0, _safeMessageTransferring.unpackMessage)(response[1]));
        break;
      case _types.PARENT_MESSAGE_MEM_USAGE:
        this._childIdleMemoryUsage = response[1];
        if (this._resolveMemoryUsage) {
          this._resolveMemoryUsage(response[1]);
          this._resolveMemoryUsage = undefined;
          this._memoryUsagePromise = undefined;
        }
        this._performRestartIfRequired();
        break;
      default:
        // Ignore messages not intended for us
        break;
    }
  }
  _performRestartIfRequired() {
    if (this._memoryUsageCheck) {
      this._memoryUsageCheck = false;
      let limit = this._childIdleMemoryUsageLimit;

      // TODO: At some point it would make sense to make use of
      // stringToBytes found in jest-config, however as this
      // package does not have any dependencies on an other jest
      // packages that can wait until some other time.
      if (limit && limit > 0 && limit <= 1) {
        limit = Math.floor((0, _nodeOs().totalmem)() * limit);
      } else if (limit) {
        limit = Math.floor(limit);
      }
      if (limit && this._childIdleMemoryUsage && this._childIdleMemoryUsage > limit) {
        this._restart();
      }
    }
  }
  _restart() {
    this.state = _types.WorkerStates.RESTARTING;
    this.killChild();
  }
  _onExit(exitCode, signal) {
    this._workerReadyPromise = undefined;
    this._resolveWorkerReady = undefined;
    this._detectOutOfMemoryCrash();
    if (exitCode !== 0 && this.state === _types.WorkerStates.OUT_OF_MEMORY) {
      this._onProcessEnd(new Error('Jest worker ran out of memory and crashed'), null);
      this._shutdown();
    } else if (exitCode !== 0 && exitCode !== null && exitCode !== SIGTERM_EXIT_CODE && exitCode !== SIGKILL_EXIT_CODE && this.state !== _types.WorkerStates.SHUTTING_DOWN || this.state === _types.WorkerStates.RESTARTING) {
      this.state = _types.WorkerStates.RESTARTING;
      this.initialize();
      if (this._request) {
        this._child.send(this._request);
      }
    } else {
      // At this point, it's not clear why the child process exited. There could
      // be several reasons:
      //
      //  1. The child process exited successfully after finishing its work.
      //     This is the most likely case.
      //  2. The child process crashed in a manner that wasn't caught through
      //     any of the heuristic-based checks above.
      //  3. The child process was killed by another process or daemon unrelated
      //     to Jest. For example, oom-killer on Linux may have picked the child
      //     process to kill because overall system memory is constrained.
      //
      // If there's a pending request to the child process in any of those
      // situations, the request still needs to be handled in some manner before
      // entering the shutdown phase. Otherwise the caller expecting a response
      // from the worker will never receive indication that something unexpected
      // happened and hang forever.
      //
      // In normal operation, the request is handled and cleared before the
      // child process exits. If it's still present, it's not clear what
      // happened and probably best to throw an error. In practice, this usually
      // happens when the child process is killed externally.
      //
      // There's a reasonable argument that the child process should be retried
      // with request re-sent in this scenario. However, if the problem was due
      // to situations such as oom-killer attempting to free up system
      // resources, retrying would exacerbate the problem.
      const isRequestStillPending = !!this._request;
      if (isRequestStillPending) {
        // If a signal is present, we can be reasonably confident the process
        // was killed externally. Log this fact so it's more clear to users that
        // something went wrong externally, rather than a bug in Jest itself.
        const error = new Error(signal == null ? `A jest worker process (pid=${this._child.pid}) crashed for an unknown reason: exitCode=${exitCode}` : `A jest worker process (pid=${this._child.pid}) was terminated by another process: signal=${signal}, exitCode=${exitCode}. Operating system logs may contain more information on why this occurred.`);
        this._onProcessEnd(error, null);
      }
      this._shutdown();
    }
  }
  send(request, onProcessStart, onProcessEnd, onCustomMessage) {
    this._stderrBuffer = [];
    onProcessStart(this);
    this._onProcessEnd = (...args) => {
      const hasRequest = !!this._request;

      // Clean the request to avoid sending past requests to workers that fail
      // while waiting for a new request (timers, unhandled rejections...)
      this._request = null;
      if (this._childIdleMemoryUsageLimit !== null && this._child.connected && hasRequest) {
        if (this._childIdleMemoryUsageLimit === 0) {
          // Special case: `idleMemoryLimit` of `0` means always restart.
          this._restart();
        } else {
          this.checkMemoryUsage();
        }
      }
      return onProcessEnd(...args);
    };
    this._onCustomMessage = (...arg) => onCustomMessage(...arg);
    this._request = request;
    this._retries = 0;
    // eslint-disable-next-line @typescript-eslint/no-empty-function
    this._child.send(request, () => {});
  }
  waitForExit() {
    return this._exitPromise;
  }
  killChild() {
    // We store a reference so that there's no way we can accidentally
    // kill a new worker that has been spawned.
    const childToKill = this._child;
    childToKill.kill('SIGTERM');
    return setTimeout(() => childToKill.kill('SIGKILL'), SIGKILL_DELAY);
  }
  forceExit() {
    this.state = _types.WorkerStates.SHUTTING_DOWN;
    const sigkillTimeout = this.killChild();
    this._exitPromise.then(() => clearTimeout(sigkillTimeout));
  }
  getWorkerId() {
    return this._options.workerId;
  }

  /**
   * Gets the process id of the worker.
   *
   * @returns Process id.
   */
  getWorkerSystemId() {
    return this._child.pid;
  }
  getStdout() {
    return this._stdout;
  }
  getStderr() {
    return this._stderr;
  }

  /**
   * Gets the last reported memory usage.
   *
   * @returns Memory usage in bytes.
   */
  getMemoryUsage() {
    if (!this._memoryUsagePromise) {
      let rejectCallback;
      const promise = new Promise((resolve, reject) => {
        this._resolveMemoryUsage = resolve;
        rejectCallback = reject;
      });
      this._memoryUsagePromise = promise;
      if (!this._child.connected && rejectCallback) {
        rejectCallback(new Error('Child process is not running.'));
        this._memoryUsagePromise = undefined;
        this._resolveMemoryUsage = undefined;
        return promise;
      }
      this._child.send([_types.CHILD_MESSAGE_MEM_USAGE], err => {
        if (err && rejectCallback) {
          this._memoryUsagePromise = undefined;
          this._resolveMemoryUsage = undefined;
          rejectCallback(err);
        }
      });
      return promise;
    }
    return this._memoryUsagePromise;
  }

  /**
   * Gets updated memory usage and restarts if required
   */
  checkMemoryUsage() {
    if (this._childIdleMemoryUsageLimit === null) {
      console.warn('Memory usage of workers can only be checked if a limit is set');
    } else {
      this._memoryUsageCheck = true;
      this._child.send([_types.CHILD_MESSAGE_MEM_USAGE], err => {
        if (err) {
          console.error('Unable to check memory usage', err);
        }
      });
    }
  }
  isWorkerRunning() {
    return this._child.connected && !this._child.killed;
  }
}
exports.Ay = ChildProcessWorker;

/***/ },

/***/ "./src/workers/NodeThreadsWorker.ts"
(__unused_webpack_module, exports, __webpack_require__) {

var __webpack_unused_export__;


__webpack_unused_export__ = ({
  value: true
});
exports.A = void 0;
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
    return data;
  };
  return data;
}
function _nodeWorker_threads() {
  const data = require("node:worker_threads");
  _nodeWorker_threads = function () {
    return data;
  };
  return data;
}
function _mergeStream() {
  const data = _interopRequireDefault(require("merge-stream"));
  _mergeStream = function () {
    return data;
  };
  return data;
}
var _types = __webpack_require__("./src/types.ts");
var _WorkerAbstract = _interopRequireDefault(__webpack_require__("./src/workers/WorkerAbstract.ts"));
var _safeMessageTransferring = __webpack_require__("./src/workers/safeMessageTransferring.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class ExperimentalWorker extends _WorkerAbstract.default {
  _worker;
  _options;
  _request;
  _retries;
  _onProcessEnd;
  _onCustomMessage;
  _stdout;
  _stderr;
  _memoryUsagePromise;
  _resolveMemoryUsage;
  _childWorkerPath;
  _childIdleMemoryUsage;
  _childIdleMemoryUsageLimit;
  _memoryUsageCheck = false;
  constructor(options) {
    super(options);
    this._options = options;
    this._request = null;
    this._stdout = null;
    this._stderr = null;
    this._childWorkerPath = options.childWorkerPath || require.resolve('./threadChild');
    this._childIdleMemoryUsage = null;
    this._childIdleMemoryUsageLimit = options.idleMemoryLimit || null;
    this.initialize();
  }
  initialize() {
    if (this.state === _types.WorkerStates.OUT_OF_MEMORY || this.state === _types.WorkerStates.SHUTTING_DOWN || this.state === _types.WorkerStates.SHUT_DOWN) {
      return;
    }
    if (this._worker) {
      this._worker.terminate();
    }
    this.state = _types.WorkerStates.STARTING;
    this._worker = new (_nodeWorker_threads().Worker)(this._childWorkerPath, {
      eval: false,
      resourceLimits: this._options.resourceLimits,
      stderr: true,
      stdout: true,
      workerData: this._options.workerData,
      ...this._options.forkOptions
    });
    if (this._worker.stdout) {
      if (!this._stdout) {
        // We need to add a permanent stream to the merged stream to prevent it
        // from ending when the subprocess stream ends
        this._stdout = (0, _mergeStream().default)(this._getFakeStream());
      }
      this._stdout.add(this._worker.stdout);
    }
    if (this._worker.stderr) {
      if (!this._stderr) {
        // We need to add a permanent stream to the merged stream to prevent it
        // from ending when the subprocess stream ends
        this._stderr = (0, _mergeStream().default)(this._getFakeStream());
      }
      this._stderr.add(this._worker.stderr);
    }

    // This can be useful for debugging.
    if (!(this._options.silent ?? true)) {
      this._worker.stdout.setEncoding('utf8');
      // eslint-disable-next-line no-console
      this._worker.stdout.on('data', console.log);
      this._worker.stderr.setEncoding('utf8');
      this._worker.stderr.on('data', console.error);
    }
    this._worker.on('message', this._onMessage.bind(this));
    this._worker.on('exit', this._onExit.bind(this));
    this._worker.on('error', this._onError.bind(this));
    this._worker.postMessage([_types.CHILD_MESSAGE_INITIALIZE, false, this._options.workerPath, this._options.setupArgs, String(this._options.workerId + 1) // 0-indexed workerId, 1-indexed JEST_WORKER_ID
    ]);
    this._retries++;

    // If we exceeded the amount of retries, we will emulate an error reply
    // coming from the child. This avoids code duplication related with cleaning
    // the queue, and scheduling the next call.
    if (this._retries > this._options.maxRetries) {
      const error = new Error('Call retries were exceeded');
      this._onMessage([_types.PARENT_MESSAGE_CLIENT_ERROR, error.name, error.message, error.stack, {
        type: 'WorkerError'
      }]);
    }
    this.state = _types.WorkerStates.OK;
    if (this._resolveWorkerReady) {
      this._resolveWorkerReady();
    }
  }
  _onError(error) {
    if (error.message.includes('heap out of memory')) {
      this.state = _types.WorkerStates.OUT_OF_MEMORY;

      // Threads don't behave like processes, they don't crash when they run out of
      // memory. But for consistency we want them to behave like processes so we call
      // terminate to simulate a crash happening that was not planned
      this._worker.terminate();
    }
  }
  _onMessage(response) {
    // Ignore messages not intended for us
    if (!Array.isArray(response)) return;
    let error;
    switch (response[0]) {
      case _types.PARENT_MESSAGE_OK:
        this._onProcessEnd(null, (0, _safeMessageTransferring.unpackMessage)(response[1]));
        break;
      case _types.PARENT_MESSAGE_CLIENT_ERROR:
        error = response[4];
        if (error != null && typeof error === 'object') {
          const extra = error;
          // @ts-expect-error: no index
          const NativeCtor = globalThis[response[1]];
          const Ctor = typeof NativeCtor === 'function' ? NativeCtor : Error;
          error = new Ctor(response[2]);
          error.type = response[1];
          error.stack = response[3];
          for (const key in extra) {
            // @ts-expect-error: no index
            error[key] = extra[key];
          }
        }
        this._onProcessEnd(error, null);
        break;
      case _types.PARENT_MESSAGE_SETUP_ERROR:
        error = new Error(`Error when calling setup: ${response[2]}`);

        // @ts-expect-error: adding custom properties to errors.
        error.type = response[1];
        error.stack = response[3];
        this._onProcessEnd(error, null);
        break;
      case _types.PARENT_MESSAGE_CUSTOM:
        this._onCustomMessage((0, _safeMessageTransferring.unpackMessage)(response[1]));
        break;
      case _types.PARENT_MESSAGE_MEM_USAGE:
        this._childIdleMemoryUsage = response[1];
        if (this._resolveMemoryUsage) {
          this._resolveMemoryUsage(response[1]);
          this._resolveMemoryUsage = undefined;
          this._memoryUsagePromise = undefined;
        }
        this._performRestartIfRequired();
        break;
      default:
        // Ignore messages not intended for us
        break;
    }
  }
  _onExit(exitCode) {
    this._workerReadyPromise = undefined;
    this._resolveWorkerReady = undefined;
    if (exitCode !== 0 && this.state === _types.WorkerStates.OUT_OF_MEMORY) {
      this._onProcessEnd(new Error('Jest worker ran out of memory and crashed'), null);
      this._shutdown();
    } else if (exitCode !== 0 && this.state !== _types.WorkerStates.SHUTTING_DOWN && this.state !== _types.WorkerStates.SHUT_DOWN || this.state === _types.WorkerStates.RESTARTING) {
      this.initialize();
      if (this._request) {
        this._worker.postMessage(this._request);
      }
    } else {
      // If the worker thread exits while a request is still pending, throw an
      // error. This is unexpected and tests may not have run to completion.
      const isRequestStillPending = !!this._request;
      if (isRequestStillPending) {
        this._onProcessEnd(new Error('A Jest worker thread exited unexpectedly before finishing tests for an unknown reason. One of the ways this can happen is if process.exit() was called in testing code.'), null);
      }
      this._shutdown();
    }
  }
  waitForExit() {
    return this._exitPromise;
  }
  forceExit() {
    this.state = _types.WorkerStates.SHUTTING_DOWN;
    this._worker.terminate();
  }
  send(request, onProcessStart, onProcessEnd, onCustomMessage) {
    onProcessStart(this);
    this._onProcessEnd = (...args) => {
      const hasRequest = !!this._request;

      // Clean the request to avoid sending past requests to workers that fail
      // while waiting for a new request (timers, unhandled rejections...)
      this._request = null;
      if (this._childIdleMemoryUsageLimit && hasRequest) {
        this.checkMemoryUsage();
      }
      const res = onProcessEnd?.(...args);

      // Clean up the reference so related closures can be garbage collected.
      onProcessEnd = null;
      return res;
    };
    this._onCustomMessage = (...arg) => onCustomMessage(...arg);
    this._request = request;
    this._retries = 0;
    this._worker.postMessage(request);
  }
  getWorkerId() {
    return this._options.workerId;
  }
  getStdout() {
    return this._stdout;
  }
  getStderr() {
    return this._stderr;
  }
  _performRestartIfRequired() {
    if (this._memoryUsageCheck) {
      this._memoryUsageCheck = false;
      let limit = this._childIdleMemoryUsageLimit;

      // TODO: At some point it would make sense to make use of
      // stringToBytes found in jest-config, however as this
      // package does not have any dependencies on an other jest
      // packages that can wait until some other time.
      if (limit && limit > 0 && limit <= 1) {
        limit = Math.floor((0, _nodeOs().totalmem)() * limit);
      } else if (limit) {
        limit = Math.floor(limit);
      }
      if (limit && this._childIdleMemoryUsage && this._childIdleMemoryUsage > limit) {
        this.state = _types.WorkerStates.RESTARTING;
        this._worker.terminate();
      }
    }
  }

  /**
   * Gets the last reported memory usage.
   *
   * @returns Memory usage in bytes.
   */
  getMemoryUsage() {
    if (!this._memoryUsagePromise) {
      let rejectCallback;
      const promise = new Promise((resolve, reject) => {
        this._resolveMemoryUsage = resolve;
        rejectCallback = reject;
      });
      this._memoryUsagePromise = promise;
      if (!this._worker.threadId) {
        rejectCallback(new Error('Child process is not running.'));
        this._memoryUsagePromise = undefined;
        this._resolveMemoryUsage = undefined;
        return promise;
      }
      try {
        this._worker.postMessage([_types.CHILD_MESSAGE_MEM_USAGE]);
      } catch (error) {
        this._memoryUsagePromise = undefined;
        this._resolveMemoryUsage = undefined;
        rejectCallback(error);
      }
      return promise;
    }
    return this._memoryUsagePromise;
  }

  /**
   * Gets updated memory usage and restarts if required
   */
  checkMemoryUsage() {
    if (this._childIdleMemoryUsageLimit) {
      this._memoryUsageCheck = true;
      this._worker.postMessage([_types.CHILD_MESSAGE_MEM_USAGE]);
    } else {
      console.warn('Memory usage of workers can only be checked if a limit is set');
    }
  }

  /**
   * Gets the thread id of the worker.
   *
   * @returns Thread id.
   */
  getWorkerSystemId() {
    return this._worker.threadId;
  }
  isWorkerRunning() {
    return this._worker.threadId >= 0;
  }
}
exports.A = ExperimentalWorker;

/***/ },

/***/ "./src/workers/WorkerAbstract.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
function _nodeStream() {
  const data = require("node:stream");
  _nodeStream = function () {
    return data;
  };
  return data;
}
var _types = __webpack_require__("./src/types.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

class WorkerAbstract extends _nodeStream().EventEmitter {
  /**
   * DO NOT WRITE TO THIS DIRECTLY.
   * Use this.state getter/setters so events are emitted correctly.
   */
  #state = _types.WorkerStates.STARTING;
  _fakeStream = null;
  _exitPromise;
  _resolveExitPromise;
  _workerReadyPromise;
  _resolveWorkerReady;
  get state() {
    return this.#state;
  }
  set state(value) {
    if (this.#state !== value) {
      const oldState = this.#state;
      this.#state = value;
      this.emit(_types.WorkerEvents.STATE_CHANGE, value, oldState);
    }
  }
  constructor(options) {
    super();
    if (typeof options.on === 'object') {
      for (const [event, handlers] of Object.entries(options.on)) {
        // Can't do Array.isArray on a ReadonlyArray<T>.
        // https://github.com/microsoft/TypeScript/issues/17002
        if (typeof handlers === 'function') {
          super.on(event, handlers);
        } else {
          for (const handler of handlers) {
            super.on(event, handler);
          }
        }
      }
    }
    this._exitPromise = new Promise(resolve => {
      this._resolveExitPromise = resolve;
    });
    this._exitPromise.then(() => {
      this.state = _types.WorkerStates.SHUT_DOWN;
    });
  }

  /**
   * Wait for the worker child process to be ready to handle requests.
   *
   * @returns Promise which resolves when ready.
   */
  waitForWorkerReady() {
    if (!this._workerReadyPromise) {
      this._workerReadyPromise = new Promise((resolve, reject) => {
        let settled = false;
        let to;
        switch (this.state) {
          case _types.WorkerStates.OUT_OF_MEMORY:
          case _types.WorkerStates.SHUTTING_DOWN:
          case _types.WorkerStates.SHUT_DOWN:
            settled = true;
            reject(new Error(`Worker state means it will never be ready: ${this.state}`));
            break;
          case _types.WorkerStates.STARTING:
          case _types.WorkerStates.RESTARTING:
            this._resolveWorkerReady = () => {
              settled = true;
              resolve();
              if (to) {
                clearTimeout(to);
              }
            };
            break;
          case _types.WorkerStates.OK:
            settled = true;
            resolve();
            break;
        }
        if (!settled) {
          to = setTimeout(() => {
            if (!settled) {
              reject(new Error('Timeout starting worker'));
            }
          }, 500);
        }
      });
    }
    return this._workerReadyPromise;
  }

  /**
   * Used to shut down the current working instance once the children have been
   * killed off.
   */
  _shutdown() {
    this.state = _types.WorkerStates.SHUT_DOWN;

    // End the permanent stream so the merged stream end too
    if (this._fakeStream) {
      this._fakeStream.end();
      this._fakeStream = null;
    }
    this._resolveExitPromise();
  }
  _getFakeStream() {
    if (!this._fakeStream) {
      this._fakeStream = new (_nodeStream().PassThrough)();
    }
    return this._fakeStream;
  }
}
exports["default"] = WorkerAbstract;

/***/ },

/***/ "./src/workers/isDataCloneError.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.isDataCloneError = isDataCloneError;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// https://webidl.spec.whatwg.org/#datacloneerror
const DATA_CLONE_ERROR_CODE = 25;

/**
 * Unfortunately, [`util.types.isNativeError(value)`](https://nodejs.org/api/util.html#utiltypesisnativeerrorvalue)
 * return `false` for `DataCloneError` error.
 * For this reason, try to detect it in this way
 */
function isDataCloneError(error) {
  return error != null && typeof error === 'object' && 'name' in error && error.name === 'DataCloneError' && 'message' in error && typeof error.message === 'string' && 'code' in error && error.code === DATA_CLONE_ERROR_CODE;
}

/***/ },

/***/ "./src/workers/messageParent.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = messageParent;
function _nodeWorker_threads() {
  const data = require("node:worker_threads");
  _nodeWorker_threads = function () {
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
var _types = __webpack_require__("./src/types.ts");
var _isDataCloneError = __webpack_require__("./src/workers/isDataCloneError.ts");
var _safeMessageTransferring = __webpack_require__("./src/workers/safeMessageTransferring.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function messageParent(message, parentProcess = process) {
  if (!_nodeWorker_threads().isMainThread && _nodeWorker_threads().parentPort != null) {
    try {
      _nodeWorker_threads().parentPort.postMessage([_types.PARENT_MESSAGE_CUSTOM, message]);
    } catch (error) {
      // Try to handle https://html.spec.whatwg.org/multipage/structured-data.html#structuredserializeinternal
      // for `symbols` and `functions`
      if ((0, _isDataCloneError.isDataCloneError)(error)) {
        _nodeWorker_threads().parentPort.postMessage([_types.PARENT_MESSAGE_CUSTOM, (0, _safeMessageTransferring.packMessage)(message)]);
      } else {
        throw error;
      }
    }
  } else if (typeof parentProcess.send === 'function') {
    try {
      parentProcess.send([_types.PARENT_MESSAGE_CUSTOM, message]);
    } catch (error) {
      if ((0, _jestUtil().isError)(error) &&
      // if .send is a function, it's a serialization issue
      !error.message.includes('.send is not a function')) {
        // Apply specific serialization only in error cases
        // to avoid affecting performance in regular cases.
        parentProcess.send([_types.PARENT_MESSAGE_CUSTOM, (0, _safeMessageTransferring.packMessage)(message)]);
      } else {
        throw error;
      }
    }
  } else {
    throw new TypeError('"messageParent" can only be used inside a worker');
  }
}

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
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
Object.defineProperty(exports, "FifoQueue", ({
  enumerable: true,
  get: function () {
    return _FifoQueue.default;
  }
}));
Object.defineProperty(exports, "PriorityQueue", ({
  enumerable: true,
  get: function () {
    return _PriorityQueue.default;
  }
}));
exports.Worker = void 0;
Object.defineProperty(exports, "messageParent", ({
  enumerable: true,
  get: function () {
    return _messageParent.default;
  }
}));
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
    return data;
  };
  return data;
}
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
var _Farm = _interopRequireDefault(__webpack_require__("./src/Farm.ts"));
var _WorkerPool = _interopRequireDefault(__webpack_require__("./src/WorkerPool.ts"));
var _PriorityQueue = _interopRequireDefault(__webpack_require__("./src/PriorityQueue.ts"));
var _FifoQueue = _interopRequireDefault(__webpack_require__("./src/FifoQueue.ts"));
var _messageParent = _interopRequireDefault(__webpack_require__("./src/workers/messageParent.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

function getExposedMethods(workerPath, options) {
  let exposedMethods = options.exposedMethods;

  // If no methods list is given, try getting it by auto-requiring the module.
  if (!exposedMethods) {
    const module = require(workerPath);
    exposedMethods = Object.keys(module).filter(name => typeof module[name] === 'function');
    if (typeof module === 'function') {
      exposedMethods = [...exposedMethods, 'default'];
    }
  }
  return exposedMethods;
}

/**
 * The Jest farm (publicly called "Worker") is a class that allows you to queue
 * methods across multiple child processes, in order to parallelize work. This
 * is done by providing an absolute path to a module that will be loaded on each
 * of the child processes, and bridged to the main process.
 *
 * Bridged methods are specified by using the "exposedMethods" property of the
 * "options" object. This is an array of strings, where each of them corresponds
 * to the exported name in the loaded module.
 *
 * You can also control the amount of workers by using the "numWorkers" property
 * of the "options" object, and the settings passed to fork the process through
 * the "forkOptions" property. The amount of workers defaults to the amount of
 * CPUS minus one.
 *
 * Queueing calls can be done in two ways:
 *   - Standard method: calls will be redirected to the first available worker,
 *     so they will get executed as soon as they can.
 *
 *   - Sticky method: if a "computeWorkerKey" method is provided within the
 *     config, the resulting string of this method will be used as a key.
 *     Every time this key is returned, it is guaranteed that your job will be
 *     processed by the same worker. This is specially useful if your workers
 *     are caching results.
 */
class Worker {
  _ending;
  _farm;
  _options;
  _workerPool;
  constructor(workerPath, options) {
    this._options = {
      ...options
    };
    this._ending = false;
    if (typeof workerPath !== 'string') {
      workerPath = workerPath.href;
    }
    if (workerPath.startsWith('file:')) {
      workerPath = (0, _nodeUrl().fileURLToPath)(workerPath);
    } else if (!(0, _nodePath().isAbsolute)(workerPath)) {
      throw new Error(`'workerPath' must be absolute, got '${workerPath}'`);
    }
    const workerPoolOptions = {
      enableWorkerThreads: this._options.enableWorkerThreads ?? false,
      forkOptions: this._options.forkOptions ?? {},
      idleMemoryLimit: this._options.idleMemoryLimit,
      maxRetries: this._options.maxRetries ?? 3,
      numWorkers: this._options.numWorkers ?? Math.max((0, _nodeOs().availableParallelism)() - 1, 1),
      resourceLimits: this._options.resourceLimits ?? {},
      setupArgs: this._options.setupArgs ?? [],
      workerGracefulExitTimeout: this._options.workerGracefulExitTimeout
    };
    if (this._options.WorkerPool) {
      this._workerPool = new this._options.WorkerPool(workerPath, workerPoolOptions);
    } else {
      this._workerPool = new _WorkerPool.default(workerPath, workerPoolOptions);
    }
    this._farm = new _Farm.default(workerPoolOptions.numWorkers, this._workerPool.send.bind(this._workerPool), {
      computeWorkerKey: this._options.computeWorkerKey,
      taskQueue: this._options.taskQueue,
      workerSchedulingPolicy: this._options.workerSchedulingPolicy
    });
    this._bindExposedWorkerMethods(workerPath, this._options);
  }
  _bindExposedWorkerMethods(workerPath, options) {
    for (const name of getExposedMethods(workerPath, options)) {
      if (name.startsWith('_')) {
        continue;
      }

      // eslint-disable-next-line no-prototype-builtins
      if (this.constructor.prototype.hasOwnProperty(name)) {
        throw new TypeError(`Cannot define a method called ${name}`);
      }

      // @ts-expect-error: dynamic extension of the class instance is expected.
      this[name] = this._callFunctionWithArgs.bind(this, name);
    }
  }
  _callFunctionWithArgs(method, ...args) {
    if (this._ending) {
      throw new Error('Farm is ended, no more calls can be done to it');
    }
    return this._farm.doWork(method, ...args);
  }
  getStderr() {
    return this._workerPool.getStderr();
  }
  getStdout() {
    return this._workerPool.getStdout();
  }
  async start() {
    await this._workerPool.start();
  }
  async end() {
    if (this._ending) {
      throw new Error('Farm is ended, no more calls can be done to it');
    }
    this._ending = true;
    return this._workerPool.end();
  }
}
exports.Worker = Worker;
})();

module.exports = __webpack_exports__;
/******/ })()
;