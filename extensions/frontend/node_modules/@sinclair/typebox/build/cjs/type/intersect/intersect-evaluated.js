"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IntersectEvaluated = IntersectEvaluated;
const index_1 = require("../symbols/index");
const type_1 = require("../create/type");
const index_2 = require("../discard/index");
const index_3 = require("../never/index");
const index_4 = require("../optional/index");
const intersect_create_1 = require("./intersect-create");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function IsIntersectOptional(types) {
    return types.every(left => (0, kind_1.IsOptional)(left));
}
// prettier-ignore
function RemoveOptionalFromType(type) {
    return ((0, index_2.Discard)(type, [index_1.OptionalKind]));
}
// prettier-ignore
function RemoveOptionalFromRest(types) {
    return types.map(left => (0, kind_1.IsOptional)(left) ? RemoveOptionalFromType(left) : left);
}
// prettier-ignore
function ResolveIntersect(types, options) {
    return (IsIntersectOptional(types)
        ? (0, index_4.Optional)((0, intersect_create_1.IntersectCreate)(RemoveOptionalFromRest(types), options))
        : (0, intersect_create_1.IntersectCreate)(RemoveOptionalFromRest(types), options));
}
/** `[Json]` Creates an evaluated Intersect type */
function IntersectEvaluated(types, options = {}) {
    if (types.length === 1)
        return (0, type_1.CreateType)(types[0], options);
    if (types.length === 0)
        return (0, index_3.Never)(options);
    if (types.some((schema) => (0, kind_1.IsTransform)(schema)))
        throw new Error('Cannot intersect transform types');
    return ResolveIntersect(types, options);
}
