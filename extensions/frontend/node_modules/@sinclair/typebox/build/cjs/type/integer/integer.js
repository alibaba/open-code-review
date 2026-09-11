"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Integer = Integer;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates an Integer type */
function Integer(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Integer', type: 'integer' }, options);
}
