"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Omit = Omit;
const type_1 = require("../create/type");
const discard_1 = require("../discard/discard");
const symbols_1 = require("../symbols/symbols");
const index_1 = require("../computed/index");
const index_2 = require("../literal/index");
const index_3 = require("../indexed/index");
const index_4 = require("../intersect/index");
const index_5 = require("../union/index");
const index_6 = require("../object/index");
// ------------------------------------------------------------------
// Mapped
// ------------------------------------------------------------------
const omit_from_mapped_key_1 = require("./omit-from-mapped-key");
const omit_from_mapped_result_1 = require("./omit-from-mapped-result");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
const value_1 = require("../guard/value");
// prettier-ignore
function FromIntersect(types, propertyKeys) {
    return types.map((type) => OmitResolve(type, propertyKeys));
}
// prettier-ignore
function FromUnion(types, propertyKeys) {
    return types.map((type) => OmitResolve(type, propertyKeys));
}
// ------------------------------------------------------------------
// FromProperty
// ------------------------------------------------------------------
// prettier-ignore
function FromProperty(properties, key) {
    const { [key]: _, ...R } = properties;
    return R;
}
// prettier-ignore
function FromProperties(properties, propertyKeys) {
    return propertyKeys.reduce((T, K2) => FromProperty(T, K2), properties);
}
// prettier-ignore
function FromObject(type, propertyKeys, properties) {
    const options = (0, discard_1.Discard)(type, [symbols_1.TransformKind, '$id', 'required', 'properties']);
    const mappedProperties = FromProperties(properties, propertyKeys);
    return (0, index_6.Object)(mappedProperties, options);
}
// prettier-ignore
function UnionFromPropertyKeys(propertyKeys) {
    const result = propertyKeys.reduce((result, key) => (0, kind_1.IsLiteralValue)(key) ? [...result, (0, index_2.Literal)(key)] : result, []);
    return (0, index_5.Union)(result);
}
// prettier-ignore
function OmitResolve(type, propertyKeys) {
    return ((0, kind_1.IsIntersect)(type) ? (0, index_4.Intersect)(FromIntersect(type.allOf, propertyKeys)) :
        (0, kind_1.IsUnion)(type) ? (0, index_5.Union)(FromUnion(type.anyOf, propertyKeys)) :
            (0, kind_1.IsObject)(type) ? FromObject(type, propertyKeys, type.properties) :
                (0, index_6.Object)({}));
}
/** `[Json]` Constructs a type whose keys are picked from the given type */
// prettier-ignore
function Omit(type, key, options) {
    const typeKey = (0, value_1.IsArray)(key) ? UnionFromPropertyKeys(key) : key;
    const propertyKeys = (0, kind_1.IsSchema)(key) ? (0, index_3.IndexPropertyKeys)(key) : key;
    const isTypeRef = (0, kind_1.IsRef)(type);
    const isKeyRef = (0, kind_1.IsRef)(key);
    return ((0, kind_1.IsMappedResult)(type) ? (0, omit_from_mapped_result_1.OmitFromMappedResult)(type, propertyKeys, options) :
        (0, kind_1.IsMappedKey)(key) ? (0, omit_from_mapped_key_1.OmitFromMappedKey)(type, key, options) :
            (isTypeRef && isKeyRef) ? (0, index_1.Computed)('Omit', [type, typeKey], options) :
                (!isTypeRef && isKeyRef) ? (0, index_1.Computed)('Omit', [type, typeKey], options) :
                    (isTypeRef && !isKeyRef) ? (0, index_1.Computed)('Omit', [type, typeKey], options) :
                        (0, type_1.CreateType)({ ...OmitResolve(type, propertyKeys), ...options }));
}
