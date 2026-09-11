"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Constructor = Constructor;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a Constructor type */
function Constructor(parameters, returns, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Constructor', type: 'Constructor', parameters, returns }, options);
}
