"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.MappedResult = MappedResult;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
// prettier-ignore
function MappedResult(properties) {
    return (0, type_1.CreateType)({
        [index_1.Kind]: 'MappedResult',
        properties
    });
}
