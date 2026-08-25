"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Literal = Literal;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a Literal type */
function Literal(value, options) {
    return (0, type_1.CreateType)({
        [index_1.Kind]: 'Literal',
        const: value,
        type: typeof value,
    }, options);
}
