"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.String = String;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a String type */
function String(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'String', type: 'string' }, options);
}
