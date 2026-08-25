"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Never = Never;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a Never type */
function Never(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Never', not: {} }, options);
}
