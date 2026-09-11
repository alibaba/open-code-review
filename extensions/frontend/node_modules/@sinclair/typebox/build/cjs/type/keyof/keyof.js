"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.KeyOfPropertyKeysToRest = KeyOfPropertyKeysToRest;
exports.KeyOf = KeyOf;
const type_1 = require("../create/type");
const index_1 = require("../literal/index");
const index_2 = require("../number/index");
const index_3 = require("../computed/index");
const index_4 = require("../ref/index");
const keyof_property_keys_1 = require("./keyof-property-keys");
const index_5 = require("../union/index");
const keyof_from_mapped_result_1 = require("./keyof-from-mapped-result");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function FromComputed(target, parameters) {
    return (0, index_3.Computed)('KeyOf', [(0, index_3.Computed)(target, parameters)]);
}
// prettier-ignore
function FromRef($ref) {
    return (0, index_3.Computed)('KeyOf', [(0, index_4.Ref)($ref)]);
}
// prettier-ignore
function KeyOfFromType(type, options) {
    const propertyKeys = (0, keyof_property_keys_1.KeyOfPropertyKeys)(type);
    const propertyKeyTypes = KeyOfPropertyKeysToRest(propertyKeys);
    const result = (0, index_5.UnionEvaluated)(propertyKeyTypes);
    return (0, type_1.CreateType)(result, options);
}
// prettier-ignore
function KeyOfPropertyKeysToRest(propertyKeys) {
    return propertyKeys.map(L => L === '[number]' ? (0, index_2.Number)() : (0, index_1.Literal)(L));
}
/** `[Json]` Creates a KeyOf type */
function KeyOf(type, options) {
    return ((0, kind_1.IsComputed)(type) ? FromComputed(type.target, type.parameters) : (0, kind_1.IsRef)(type) ? FromRef(type.$ref) : (0, kind_1.IsMappedResult)(type) ? (0, keyof_from_mapped_result_1.KeyOfFromMappedResult)(type, options) : KeyOfFromType(type, options));
}
