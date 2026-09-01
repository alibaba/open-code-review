"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Not = Not;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a Not type */
function Not(type, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Not', not: type }, options);
}
