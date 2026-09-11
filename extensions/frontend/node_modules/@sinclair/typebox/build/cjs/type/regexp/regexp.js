"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.RegExp = RegExp;
const type_1 = require("../create/type");
const value_1 = require("../guard/value");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a RegExp type */
function RegExp(unresolved, options) {
    const expr = (0, value_1.IsString)(unresolved) ? new globalThis.RegExp(unresolved) : unresolved;
    return (0, type_1.CreateType)({ [index_1.Kind]: 'RegExp', type: 'RegExp', source: expr.source, flags: expr.flags }, options);
}
