"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.MappedKey = MappedKey;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
// prettier-ignore
function MappedKey(T) {
    return (0, type_1.CreateType)({
        [index_1.Kind]: 'MappedKey',
        keys: T
    });
}
