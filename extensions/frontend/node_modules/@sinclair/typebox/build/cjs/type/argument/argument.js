"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Argument = Argument;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates an Argument Type. */
function Argument(index) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Argument', index });
}
