"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Uint8Array = Uint8Array;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a Uint8Array type */
function Uint8Array(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Uint8Array', type: 'Uint8Array' }, options);
}
