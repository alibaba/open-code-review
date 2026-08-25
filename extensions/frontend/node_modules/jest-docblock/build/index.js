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
exports.extract = extract;
exports.parse = parse;
exports.parseWithComments = parseWithComments;
exports.print = print;
exports.strip = strip;
function _nodeOs() {
  const data = require("node:os");
  _nodeOs = function () {
    return data;
  };
  return data;
}
function _detectNewline() {
  const data = _interopRequireDefault(require("detect-newline"));
  _detectNewline = function () {
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

const commentEndRe = /\*\/$/;
const commentStartRe = /^\/\*\*?/;
const docblockRe = /^\s*(\/\*\*?(.|\r?\n)*?\*\/)/;
const lineCommentRe = /(^|\s+)\/\/([^\n\r]*)/g;
const ltrimNewlineRe = /^(\r?\n)+/;
const multilineRe = /(?:^|\r?\n) *(@[^\n\r]*?) *\r?\n *(?![^\n\r@]*\/\/[^]*)([^\s@][^\n\r@]+?) *\r?\n/g;
const propertyRe = /(?:^|\r?\n) *@(\S+) *([^\n\r]*)/g;
const stringStartRe = /(\r?\n|^) *\* ?/g;
const STRING_ARRAY = [];
function extract(contents) {
  const match = contents.match(docblockRe);
  return match ? match[0].trimStart() : '';
}
function strip(contents) {
  const matchResult = contents.match(docblockRe);
  const match = matchResult?.[0];
  return match == null ? contents : contents.slice(match.length);
}
function parse(docblock) {
  return parseWithComments(docblock).pragmas;
}
function parseWithComments(docblock) {
  const line = (0, _detectNewline().default)(docblock) ?? _nodeOs().EOL;
  docblock = docblock.replace(commentStartRe, '').replace(commentEndRe, '').replaceAll(stringStartRe, '$1');

  // Normalize multi-line directives
  let prev = '';
  while (prev !== docblock) {
    prev = docblock;
    docblock = docblock.replaceAll(multilineRe, `${line}$1 $2${line}`);
  }
  docblock = docblock.replace(ltrimNewlineRe, '').trimEnd();
  const result = Object.create(null);
  const comments = docblock.replaceAll(propertyRe, '').replace(ltrimNewlineRe, '').trimEnd();
  let match;
  while (match = propertyRe.exec(docblock)) {
    // strip linecomments from pragmas
    const nextPragma = match[2].replaceAll(lineCommentRe, '');
    if (typeof result[match[1]] === 'string' || Array.isArray(result[match[1]])) {
      const resultElement = result[match[1]];
      result[match[1]] = [...STRING_ARRAY, ...(Array.isArray(resultElement) ? resultElement : [resultElement]), nextPragma];
    } else {
      result[match[1]] = nextPragma;
    }
  }
  return {
    comments,
    pragmas: result
  };
}
function print({
  comments = '',
  pragmas = {}
}) {
  const line = (0, _detectNewline().default)(comments) ?? _nodeOs().EOL;
  const head = '/**';
  const start = ' *';
  const tail = ' */';
  const keys = Object.keys(pragmas);
  const printedObject = keys.flatMap(key => printKeyValues(key, pragmas[key])).map(keyValue => `${start} ${keyValue}${line}`).join('');
  if (!comments) {
    if (keys.length === 0) {
      return '';
    }
    if (keys.length === 1 && !Array.isArray(pragmas[keys[0]])) {
      const value = pragmas[keys[0]];
      return `${head} ${printKeyValues(keys[0], value)[0]}${tail}`;
    }
  }
  const printedComments = comments.split(line).map(textLine => `${start} ${textLine}`).join(line) + line;
  return head + line + (comments ? printedComments : '') + (comments && keys.length > 0 ? start + line : '') + printedObject + tail;
}
function printKeyValues(key, valueOrArray) {
  return [...STRING_ARRAY, ...(Array.isArray(valueOrArray) ? valueOrArray : [valueOrArray])].map(value => `@${key} ${value}`.trim());
}
})();

module.exports = __webpack_exports__;
/******/ })()
;