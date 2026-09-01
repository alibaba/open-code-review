"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Undefined = Undefined;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a Undefined type */
function Undefined(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Undefined', type: 'undefined' }, options);
}
