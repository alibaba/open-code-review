"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Symbol = Symbol;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a Symbol type */
function Symbol(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Symbol', type: 'symbol' }, options);
}
