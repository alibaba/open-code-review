"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Awaited = Awaited;
const type_1 = require("../create/type");
const index_1 = require("../computed/index");
const index_2 = require("../intersect/index");
const index_3 = require("../union/index");
const index_4 = require("../ref/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function FromComputed(target, parameters) {
    return (0, index_1.Computed)('Awaited', [(0, index_1.Computed)(target, parameters)]);
}
// prettier-ignore
function FromRef($ref) {
    return (0, index_1.Computed)('Awaited', [(0, index_4.Ref)($ref)]);
}
// prettier-ignore
function FromIntersect(types) {
    return (0, index_2.Intersect)(FromRest(types));
}
// prettier-ignore
function FromUnion(types) {
    return (0, index_3.Union)(FromRest(types));
}
// prettier-ignore
function FromPromise(type) {
    return Awaited(type);
}
// prettier-ignore
function FromRest(types) {
    return types.map(type => Awaited(type));
}
/** `[JavaScript]` Constructs a type by recursively unwrapping Promise types */
function Awaited(type, options) {
    return (0, type_1.CreateType)((0, kind_1.IsComputed)(type) ? FromComputed(type.target, type.parameters) : (0, kind_1.IsIntersect)(type) ? FromIntersect(type.allOf) : (0, kind_1.IsUnion)(type) ? FromUnion(type.anyOf) : (0, kind_1.IsPromise)(type) ? FromPromise(type.item) : (0, kind_1.IsRef)(type) ? FromRef(type.$ref) : type, options);
}
