"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.UnionEvaluated = UnionEvaluated;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
const index_2 = require("../discard/index");
const index_3 = require("../never/index");
const index_4 = require("../optional/index");
const union_create_1 = require("./union-create");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function IsUnionOptional(types) {
    return types.some(type => (0, kind_1.IsOptional)(type));
}
// prettier-ignore
function RemoveOptionalFromRest(types) {
    return types.map(left => (0, kind_1.IsOptional)(left) ? RemoveOptionalFromType(left) : left);
}
// prettier-ignore
function RemoveOptionalFromType(T) {
    return ((0, index_2.Discard)(T, [index_1.OptionalKind]));
}
// prettier-ignore
function ResolveUnion(types, options) {
    const isOptional = IsUnionOptional(types);
    return (isOptional
        ? (0, index_4.Optional)((0, union_create_1.UnionCreate)(RemoveOptionalFromRest(types), options))
        : (0, union_create_1.UnionCreate)(RemoveOptionalFromRest(types), options));
}
/** `[Json]` Creates an evaluated Union type */
function UnionEvaluated(T, options) {
    // prettier-ignore
    return (T.length === 1 ? (0, type_1.CreateType)(T[0], options) :
        T.length === 0 ? (0, index_3.Never)(options) :
            ResolveUnion(T, options));
}
