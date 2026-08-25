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

/***/ "./src/bind.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = bind;
function _jestUtil() {
  const data = require("jest-util");
  _jestUtil = function () {
    return data;
  };
  return data;
}
var _array = _interopRequireDefault(__webpack_require__("./src/table/array.ts"));
var _template = _interopRequireDefault(__webpack_require__("./src/table/template.ts"));
var _validation = __webpack_require__("./src/validation.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

// type TestFn = (done?: Global.DoneFn) => Promise<any> | void | undefined;

function bind(cb, supportsDone = true, needsEachError = false) {
  const bindWrap = (table, ...taggedTemplateData) => {
    const errorWithStack = new (_jestUtil().ErrorWithStack)(undefined, bindWrap);
    return function eachBind(title, test, timeout) {
      title = (0, _jestUtil().convertDescriptorToString)(title);
      try {
        const tests = isArrayTable(taggedTemplateData) ? buildArrayTests(title, table) : buildTemplateTests(title, table, taggedTemplateData);
        for (const row of tests) {
          if (needsEachError) {
            cb(row.title, applyArguments(supportsDone, row.arguments, test), timeout, errorWithStack);
          } else {
            cb(row.title, applyArguments(supportsDone, row.arguments, test), timeout);
          }
        }
        return;
      } catch (error) {
        const err = new Error(error.message);
        err.stack = errorWithStack.stack?.replace(/^Error: /s, `Error: ${error.message}`);
        return cb(title, () => {
          throw err;
        });
      }
    };
  };
  return bindWrap;
}
const isArrayTable = data => data.length === 0;
const buildArrayTests = (title, table) => {
  (0, _validation.validateArrayTable)(table);
  return (0, _array.default)(title, table);
};
const buildTemplateTests = (title, table, taggedTemplateData) => {
  const headings = getHeadingKeys(table[0]);
  (0, _validation.validateTemplateTableArguments)(headings, taggedTemplateData);
  return (0, _template.default)(title, headings, taggedTemplateData);
};
const getHeadingKeys = headings => (0, _validation.extractValidTemplateHeadings)(headings).replaceAll(/\s/g, '').split('|');
const applyArguments = (supportsDone, params, test) => supportsDone && params.length < test.length ? done => test(...params, done) : () => test(...params);

/***/ },

/***/ "./src/table/array.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = array;
function util() {
  const data = _interopRequireWildcard(require("node:util"));
  util = function () {
    return data;
  };
  return data;
}
function _prettyFormat() {
  const data = require("pretty-format");
  _prettyFormat = function () {
    return data;
  };
  return data;
}
var _interpolation = __webpack_require__("./src/table/interpolation.ts");
function _interopRequireWildcard(e, t) { if ("function" == typeof WeakMap) var r = new WeakMap(), n = new WeakMap(); return (_interopRequireWildcard = function (e, t) { if (!t && e && e.__esModule) return e; var o, i, f = { __proto__: null, default: e }; if (null === e || "object" != typeof e && "function" != typeof e) return f; if (o = t ? n : r) { if (o.has(e)) return o.get(e); o.set(e, f); } for (const t in e) "default" !== t && {}.hasOwnProperty.call(e, t) && ((i = (o = Object.defineProperty) && Object.getOwnPropertyDescriptor(e, t)) && (i.get || i.set) ? o(f, t, i) : f[t] = e[t]); return f; })(e, t); }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const SUPPORTED_PLACEHOLDERS = /%[#Odfijops]/g;
const PRETTY_PLACEHOLDER = '%p';
const INDEX_PLACEHOLDER = '%#';
const NUMBER_PLACEHOLDER = '%$';
const PLACEHOLDER_PREFIX = '%';
const ESCAPED_PLACEHOLDER_PREFIX = '%%';
const JEST_EACH_PLACEHOLDER_ESCAPE = '@@__JEST_EACH_PLACEHOLDER_ESCAPE__@@';
function array(title, arrayTable) {
  if (isTemplates(title, arrayTable)) {
    return arrayTable.map((template, index) => ({
      arguments: [template],
      title: (0, _interpolation.interpolateVariables)(title, template, index).replaceAll(ESCAPED_PLACEHOLDER_PREFIX, PLACEHOLDER_PREFIX)
    }));
  }
  return normaliseTable(arrayTable).map((row, index) => ({
    arguments: row,
    title: formatTitle(title, row, index)
  }));
}
const isTemplates = (title, arrayTable) => !SUPPORTED_PLACEHOLDERS.test(interpolateEscapedPlaceholders(title)) && !isTable(arrayTable) && arrayTable.every(col => col != null && typeof col === 'object');
const normaliseTable = table => isTable(table) ? table : table.map(colToRow);
const isTable = table => table.every(Array.isArray);
const colToRow = col => [col];
const formatTitle = (title, row, rowIndex) => row.reduce((formattedTitle, value) => {
  const [placeholder] = getMatchingPlaceholders(formattedTitle);
  const normalisedValue = normalisePlaceholderValue(value);
  if (!placeholder) return formattedTitle;
  if (placeholder === PRETTY_PLACEHOLDER) return interpolatePrettyPlaceholder(formattedTitle, normalisedValue);
  return util().format(formattedTitle, normalisedValue);
}, interpolateTitleIndexAndNumber(interpolateEscapedPlaceholders(title), rowIndex)).replaceAll(JEST_EACH_PLACEHOLDER_ESCAPE, PLACEHOLDER_PREFIX);
const normalisePlaceholderValue = value => typeof value === 'string' ? value.replaceAll(PLACEHOLDER_PREFIX, JEST_EACH_PLACEHOLDER_ESCAPE) : value;
const getMatchingPlaceholders = title => title.match(SUPPORTED_PLACEHOLDERS) || [];
const interpolateEscapedPlaceholders = title => title.replaceAll(ESCAPED_PLACEHOLDER_PREFIX, JEST_EACH_PLACEHOLDER_ESCAPE);
const interpolateTitleIndexAndNumber = (title, index) => title.replace(INDEX_PLACEHOLDER, index.toString()).replace(NUMBER_PLACEHOLDER, (index + 1).toString());
const interpolatePrettyPlaceholder = (title, value) => title.replace(PRETTY_PLACEHOLDER, (0, _prettyFormat().format)(value, {
  maxDepth: 1,
  min: true
}));

/***/ },

/***/ "./src/table/interpolation.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.getPath = getPath;
exports.interpolateVariables = void 0;
function _getType() {
  const data = require("@jest/get-type");
  _getType = function () {
    return data;
  };
  return data;
}
function _prettyFormat() {
  const data = require("pretty-format");
  _prettyFormat = function () {
    return data;
  };
  return data;
}
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const interpolateVariables = (title, template, index) => title.replaceAll(new RegExp(`\\$(${Object.keys(template).join('|')})[.\\w]*`, 'g'), match => {
  const keyPath = match.slice(1).split('.');
  const value = getPath(template, keyPath);
  return (0, _getType().isPrimitive)(value) ? String(value) : (0, _prettyFormat().format)(value, {
    maxDepth: 1,
    min: true
  });
}).replace('$#', `${index}`);

/* eslint import-x/export: 0*/
exports.interpolateVariables = interpolateVariables;
function getPath(template, [head, ...tail]) {
  if (template === null) return 'null';
  if (template === undefined) return 'undefined';
  if (!head || !Object.prototype.hasOwnProperty.call(template, head)) return template;
  return getPath(template[head], tail);
}

/***/ },

/***/ "./src/table/template.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = template;
var _interpolation = __webpack_require__("./src/table/interpolation.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

function template(title, headings, row) {
  const table = convertRowToTable(row, headings);
  const templates = convertTableToTemplates(table, headings);
  return templates.map((template, index) => ({
    arguments: [template],
    title: (0, _interpolation.interpolateVariables)(title, template, index)
  }));
}
const convertRowToTable = (row, headings) => Array.from({
  length: row.length / headings.length
}, (_, index) => row.slice(index * headings.length, index * headings.length + headings.length));
const convertTableToTemplates = (table, headings) => table.map(row => Object.fromEntries(row.map((value, index) => [headings[index], value])));

/***/ },

/***/ "./src/validation.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.validateTemplateTableArguments = exports.validateArrayTable = exports.extractValidTemplateHeadings = void 0;
function _chalk() {
  const data = _interopRequireDefault(require("chalk"));
  _chalk = function () {
    return data;
  };
  return data;
}
function _prettyFormat() {
  const data = require("pretty-format");
  _prettyFormat = function () {
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
 *
 */

const EXPECTED_COLOR = _chalk().default.green;
const RECEIVED_COLOR = _chalk().default.red;
const validateArrayTable = table => {
  if (!Array.isArray(table)) {
    throw new TypeError('`.each` must be called with an Array or Tagged Template Literal.\n\n' + `Instead was called with: ${(0, _prettyFormat().format)(table, {
      maxDepth: 1,
      min: true
    })}\n`);
  }
  if (isTaggedTemplateLiteral(table)) {
    if (isEmptyString(table[0])) {
      throw new Error('Error: `.each` called with an empty Tagged Template Literal of table data.\n');
    }
    throw new Error('Error: `.each` called with a Tagged Template Literal with no data, remember to interpolate with ${expression} syntax.\n');
  }
  if (isEmptyTable(table)) {
    throw new Error('Error: `.each` called with an empty Array of table data.\n');
  }
};
exports.validateArrayTable = validateArrayTable;
const isTaggedTemplateLiteral = array => array.raw !== undefined;
const isEmptyTable = table => table.length === 0;
const isEmptyString = str => typeof str === 'string' && str.trim() === '';
const validateTemplateTableArguments = (headings, data) => {
  const incompleteData = data.length % headings.length;
  const missingData = headings.length - incompleteData;
  if (incompleteData > 0) {
    throw new Error(`Not enough arguments supplied for given headings:\n${EXPECTED_COLOR(headings.join(' | '))}\n\n` + `Received:\n${RECEIVED_COLOR((0, _prettyFormat().format)(data))}\n\n` + `Missing ${RECEIVED_COLOR(missingData.toString())} ${pluralize('argument', missingData)}`);
  }
};
exports.validateTemplateTableArguments = validateTemplateTableArguments;
const pluralize = (word, count) => word + (count === 1 ? '' : 's');
const START_OF_LINE = '^';
const NEWLINE = '\\n';
const HEADING = '\\s*[^\\s]+\\s*';
const PIPE = '\\|';
const REPEATABLE_HEADING = `(${PIPE}${HEADING})*`;
const HEADINGS_FORMAT = new RegExp(START_OF_LINE + NEWLINE + HEADING + REPEATABLE_HEADING + NEWLINE, 'g');
const extractValidTemplateHeadings = headings => {
  const matches = headings.match(HEADINGS_FORMAT);
  if (matches === null) {
    throw new Error(`Table headings do not conform to expected format:\n\n${EXPECTED_COLOR('heading1 | headingN')}\n\nReceived:\n\n${RECEIVED_COLOR((0, _prettyFormat().format)(headings))}`);
  }
  return matches[0];
};
exports.extractValidTemplateHeadings = extractValidTemplateHeadings;

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
Object.defineProperty(exports, "bind", ({
  enumerable: true,
  get: function () {
    return _bind.default;
  }
}));
exports["default"] = void 0;
var _bind = _interopRequireDefault(__webpack_require__("./src/bind.ts"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

const install = (g, table, ...data) => {
  const bindingWithArray = data.length === 0;
  const bindingWithTemplate = Array.isArray(table) && !!table.raw;
  if (!bindingWithArray && !bindingWithTemplate) {
    throw new Error('`.each` must only be called with an Array or Tagged Template Literal.');
  }
  const test = (title, test, timeout) => (0, _bind.default)(g.test)(table, ...data)(title, test, timeout);
  test.skip = (0, _bind.default)(g.test.skip)(table, ...data);
  test.only = (0, _bind.default)(g.test.only)(table, ...data);
  const testConcurrent = (title, test, timeout) => (0, _bind.default)(g.test.concurrent)(table, ...data)(title, test, timeout);
  test.concurrent = testConcurrent;
  testConcurrent.only = (0, _bind.default)(g.test.concurrent.only)(table, ...data);
  testConcurrent.skip = (0, _bind.default)(g.test.concurrent.skip)(table, ...data);
  const it = (title, test, timeout) => (0, _bind.default)(g.it)(table, ...data)(title, test, timeout);
  it.skip = (0, _bind.default)(g.it.skip)(table, ...data);
  it.only = (0, _bind.default)(g.it.only)(table, ...data);
  it.concurrent = testConcurrent;
  const xit = (0, _bind.default)(g.xit)(table, ...data);
  const fit = (0, _bind.default)(g.fit)(table, ...data);
  const xtest = (0, _bind.default)(g.xtest)(table, ...data);
  const describe = (title, suite, timeout) => (0, _bind.default)(g.describe, false)(table, ...data)(title, suite, timeout);
  describe.skip = (0, _bind.default)(g.describe.skip, false)(table, ...data);
  describe.only = (0, _bind.default)(g.describe.only, false)(table, ...data);
  const fdescribe = (0, _bind.default)(g.fdescribe, false)(table, ...data);
  const xdescribe = (0, _bind.default)(g.xdescribe, false)(table, ...data);
  return {
    describe,
    fdescribe,
    fit,
    it,
    test,
    xdescribe,
    xit,
    xtest
  };
};
const each = (table, ...data) => install(globalThis, table, ...data);
each.withGlobal = g => (table, ...data) => install(g, table, ...data);
var _default = exports["default"] = each;
})();

module.exports = __webpack_exports__;
/******/ })()
;