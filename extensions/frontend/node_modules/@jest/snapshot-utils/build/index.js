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

/***/ "./src/index.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
var _utils = __webpack_require__("./src/utils.ts");
Object.keys(_utils).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (key in exports && exports[key] === _utils[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _utils[key];
    }
  });
});
var _types = __webpack_require__("./src/types.ts");
Object.keys(_types).forEach(function (key) {
  if (key === "default" || key === "__esModule") return;
  if (key in exports && exports[key] === _types[key]) return;
  Object.defineProperty(exports, key, {
    enumerable: true,
    get: function () {
      return _types[key];
    }
  });
});

/***/ },

/***/ "./src/types.ts"
() {



/***/ },

/***/ "./src/utils.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.testNameToKey = exports.saveSnapshotFile = exports.normalizeNewlines = exports.keyToTestName = exports.getSnapshotData = exports.escapeBacktickString = exports.ensureDirectoryExists = exports.SNAPSHOT_VERSION_WARNING = exports.SNAPSHOT_VERSION = exports.SNAPSHOT_GUIDE_LINK = void 0;
var path = _interopRequireWildcard(require("node:path"));
var _chalk = _interopRequireDefault(require("chalk"));
var fs = _interopRequireWildcard(require("graceful-fs"));
var _naturalCompare = _interopRequireDefault(require("natural-compare"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestWriteFile = globalThis[Symbol.for('jest-native-write-file')] || fs.writeFileSync;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestReadFile = globalThis[Symbol.for('jest-native-read-file')] || fs.readFileSync;
var Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
var jestExistsFile = globalThis[Symbol.for('jest-native-exists-file')] || fs.existsSync;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const SNAPSHOT_VERSION = exports.SNAPSHOT_VERSION = '1';
const SNAPSHOT_HEADER_REGEXP = /^\/\/ Jest Snapshot v(.+), (.+)$/m;
const SNAPSHOT_GUIDE_LINK = exports.SNAPSHOT_GUIDE_LINK = 'https://jestjs.io/docs/snapshot-testing';
const SNAPSHOT_VERSION_WARNING = exports.SNAPSHOT_VERSION_WARNING = _chalk.default.yellow(`${_chalk.default.bold('Warning')}: Before you upgrade snapshots, ` + 'we recommend that you revert any local changes to tests or other code, ' + 'to ensure that you do not store invalid state.');
const writeSnapshotVersion = () => `// Jest Snapshot v${SNAPSHOT_VERSION}, ${SNAPSHOT_GUIDE_LINK}`;
const validateSnapshotHeader = snapshotContents => {
  const headerTest = SNAPSHOT_HEADER_REGEXP.exec(snapshotContents);
  const version = headerTest && headerTest[1];
  const guideLink = headerTest && headerTest[2];
  if (!version) {
    return new Error(_chalk.default.red(`${_chalk.default.bold('Outdated snapshot')}: No snapshot header found. ` + 'Jest 19 introduced versioned snapshots to ensure all developers ' + 'on a project are using the same version of Jest. ' + 'Please update all snapshots during this upgrade of Jest.\n\n') + SNAPSHOT_VERSION_WARNING);
  }
  if (version < SNAPSHOT_VERSION) {
    return new Error(
    // eslint-disable-next-line prefer-template
    _chalk.default.red(`${_chalk.default.red.bold('Outdated snapshot')}: The version of the snapshot ` + 'file associated with this test is outdated. The snapshot file ' + 'version ensures that all developers on a project are using ' + 'the same version of Jest. ' + 'Please update all snapshots during this upgrade of Jest.') + '\n\n' + `Expected: v${SNAPSHOT_VERSION}\n` + `Received: v${version}\n\n` + SNAPSHOT_VERSION_WARNING);
  }
  if (version > SNAPSHOT_VERSION) {
    return new Error(
    // eslint-disable-next-line prefer-template
    _chalk.default.red(`${_chalk.default.red.bold('Outdated Jest version')}: The version of this ` + 'snapshot file indicates that this project is meant to be used ' + 'with a newer version of Jest. The snapshot file version ensures ' + 'that all developers on a project are using the same version of ' + 'Jest. Please update your version of Jest and re-run the tests.') + '\n\n' + `Expected: v${SNAPSHOT_VERSION}\n` + `Received: v${version}`);
  }
  if (guideLink !== SNAPSHOT_GUIDE_LINK) {
    return new Error(
    // eslint-disable-next-line prefer-template
    _chalk.default.red(`${_chalk.default.red.bold('Outdated guide link')}: The snapshot guide link at the top of this snapshot is outdated. ` + 'Please update all snapshots during this upgrade of Jest.') + '\n\n' + `Expected: ${SNAPSHOT_GUIDE_LINK}\n` + `Received: ${guideLink}`);
  }
  return null;
};
const normalizeTestNameForKey = testName => testName.replaceAll(/\r\n|\r|\n/g, match => {
  switch (match) {
    case '\r\n':
      return '\\r\\n';
    case '\r':
      return '\\r';
    case '\n':
      return '\\n';
    default:
      return match;
  }
});
const denormalizeTestNameFromKey = key => key.replaceAll(/\\r\\n|\\r|\\n/g, match => {
  switch (match) {
    case '\\r\\n':
      return '\r\n';
    case '\\r':
      return '\r';
    case '\\n':
      return '\n';
    default:
      return match;
  }
});
const testNameToKey = (testName, count) => `${normalizeTestNameForKey(testName)} ${count}`;
exports.testNameToKey = testNameToKey;
const keyToTestName = key => {
  if (!/ \d+$/.test(key)) {
    throw new Error('Snapshot keys must end with a number.');
  }
  const testNameWithoutCount = key.replace(/ \d+$/, '');
  return denormalizeTestNameFromKey(testNameWithoutCount);
};
exports.keyToTestName = keyToTestName;
const getSnapshotData = (snapshotPath, update) => {
  const data = Object.create(null);
  let snapshotContents = '';
  let dirty = false;
  if (jestExistsFile(snapshotPath)) {
    try {
      snapshotContents = jestReadFile(snapshotPath, 'utf8');
      // eslint-disable-next-line no-new-func
      const populate = new Function('exports', snapshotContents);
      populate(data);
    } catch {}
  }
  const validationResult = validateSnapshotHeader(snapshotContents);
  const isInvalid = snapshotContents && validationResult;
  if (update === 'none' && isInvalid) {
    throw validationResult;
  }
  if ((update === 'all' || update === 'new') && isInvalid) {
    dirty = true;
  }
  return {
    data,
    dirty
  };
};
exports.getSnapshotData = getSnapshotData;
const escapeBacktickString = str => str.replaceAll(/`|\\|\${/g, '\\$&');
exports.escapeBacktickString = escapeBacktickString;
const printBacktickString = str => `\`${escapeBacktickString(str)}\``;
const ensureDirectoryExists = filePath => {
  try {
    fs.mkdirSync(path.dirname(filePath), {
      recursive: true
    });
  } catch {}
};
exports.ensureDirectoryExists = ensureDirectoryExists;
const normalizeNewlines = string => string.replaceAll(/\r\n|\r/g, '\n');
exports.normalizeNewlines = normalizeNewlines;
const saveSnapshotFile = (snapshotData, snapshotPath) => {
  const snapshots = Object.keys(snapshotData).sort(_naturalCompare.default).map(key => `exports[${printBacktickString(key)}] = ${printBacktickString(normalizeNewlines(snapshotData[key]))};`);
  ensureDirectoryExists(snapshotPath);
  jestWriteFile(snapshotPath, `${writeSnapshotVersion()}\n\n${snapshots.join('\n\n')}\n`);
};
exports.saveSnapshotFile = saveSnapshotFile;

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