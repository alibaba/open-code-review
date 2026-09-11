"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Enum = Enum;
const index_1 = require("../literal/index");
const index_2 = require("../symbols/index");
const index_3 = require("../union/index");
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const value_1 = require("../guard/value");
/** `[Json]` Creates a Enum type */
function Enum(item, options) {
    if ((0, value_1.IsUndefined)(item))
        throw new Error('Enum undefined or empty');
    const values1 = globalThis.Object.getOwnPropertyNames(item)
        .filter((key) => isNaN(key))
        .map((key) => item[key]);
    const values2 = [...new Set(values1)];
    const anyOf = values2.map((value) => (0, index_1.Literal)(value));
    return (0, index_3.Union)(anyOf, { ...options, [index_2.Hint]: 'Enum' });
}
