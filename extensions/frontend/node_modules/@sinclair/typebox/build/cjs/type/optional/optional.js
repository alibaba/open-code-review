"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Optional = Optional;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
const index_2 = require("../discard/index");
const optional_from_mapped_result_1 = require("./optional-from-mapped-result");
const kind_1 = require("../guard/kind");
function RemoveOptional(schema) {
    return (0, type_1.CreateType)((0, index_2.Discard)(schema, [index_1.OptionalKind]));
}
function AddOptional(schema) {
    return (0, type_1.CreateType)({ ...schema, [index_1.OptionalKind]: 'Optional' });
}
// prettier-ignore
function OptionalWithFlag(schema, F) {
    return (F === false
        ? RemoveOptional(schema)
        : AddOptional(schema));
}
/** `[Json]` Creates a Optional property */
function Optional(schema, enable) {
    const F = enable ?? true;
    return (0, kind_1.IsMappedResult)(schema) ? (0, optional_from_mapped_result_1.OptionalFromMappedResult)(schema, F) : OptionalWithFlag(schema, F);
}
