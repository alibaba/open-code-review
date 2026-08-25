"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Boolean = Boolean;
const index_1 = require("../symbols/index");
const index_2 = require("../create/index");
/** `[Json]` Creates a Boolean type */
function Boolean(options) {
    return (0, index_2.CreateType)({ [index_1.Kind]: 'Boolean', type: 'boolean' }, options);
}
