"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Array = Array;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates an Array type */
function Array(items, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Array', type: 'array', items }, options);
}
