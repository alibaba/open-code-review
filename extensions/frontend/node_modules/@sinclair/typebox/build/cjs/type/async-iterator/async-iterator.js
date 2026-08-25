"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.AsyncIterator = AsyncIterator;
const index_1 = require("../symbols/index");
const type_1 = require("../create/type");
/** `[JavaScript]` Creates a AsyncIterator type */
function AsyncIterator(items, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'AsyncIterator', type: 'AsyncIterator', items }, options);
}
