"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Null = Null;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a Null type */
function Null(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Null', type: 'null' }, options);
}
