"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Function = Function;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a Function type */
function Function(parameters, returns, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Function', type: 'Function', parameters, returns }, options);
}
