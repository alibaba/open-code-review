"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.UnionCreate = UnionCreate;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
function UnionCreate(T, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Union', anyOf: T }, options);
}
