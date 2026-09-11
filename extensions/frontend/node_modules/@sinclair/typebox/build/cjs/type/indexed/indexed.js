"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IndexFromPropertyKey = IndexFromPropertyKey;
exports.IndexFromPropertyKeys = IndexFromPropertyKeys;
exports.IndexFromComputed = IndexFromComputed;
exports.Index = Index;
const type_1 = require("../create/type");
const index_1 = require("../error/index");
const index_2 = require("../computed/index");
const index_3 = require("../never/index");
const index_4 = require("../intersect/index");
const index_5 = require("../union/index");
const indexed_property_keys_1 = require("./indexed-property-keys");
const indexed_from_mapped_key_1 = require("./indexed-from-mapped-key");
const indexed_from_mapped_result_1 = require("./indexed-from-mapped-result");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function FromRest(types, key) {
    return types.map(type => IndexFromPropertyKey(type, key));
}
// prettier-ignore
function FromIntersectRest(types) {
    return types.filter(type => !(0, kind_1.IsNever)(type));
}
// prettier-ignore
function FromIntersect(types, key) {
    return ((0, index_4.IntersectEvaluated)(FromIntersectRest(FromRest(types, key))));
}
// prettier-ignore
function FromUnionRest(types) {
    return (types.some(L => (0, kind_1.IsNever)(L))
        ? []
        : types);
}
// prettier-ignore
function FromUnion(types, key) {
    return ((0, index_5.UnionEvaluated)(FromUnionRest(FromRest(types, key))));
}
// prettier-ignore
function FromTuple(types, key) {
    return (key in types ? types[key] :
        key === '[number]' ? (0, index_5.UnionEvaluated)(types) :
            (0, index_3.Never)());
}
// prettier-ignore
function FromArray(type, key) {
    return (key === '[number]'
        ? type
        : (0, index_3.Never)());
}
// prettier-ignore
function FromProperty(properties, propertyKey) {
    return (propertyKey in properties ? properties[propertyKey] : (0, index_3.Never)());
}
// prettier-ignore
function IndexFromPropertyKey(type, propertyKey) {
    return ((0, kind_1.IsIntersect)(type) ? FromIntersect(type.allOf, propertyKey) :
        (0, kind_1.IsUnion)(type) ? FromUnion(type.anyOf, propertyKey) :
            (0, kind_1.IsTuple)(type) ? FromTuple(type.items ?? [], propertyKey) :
                (0, kind_1.IsArray)(type) ? FromArray(type.items, propertyKey) :
                    (0, kind_1.IsObject)(type) ? FromProperty(type.properties, propertyKey) :
                        (0, index_3.Never)());
}
// prettier-ignore
function IndexFromPropertyKeys(type, propertyKeys) {
    return propertyKeys.map(propertyKey => IndexFromPropertyKey(type, propertyKey));
}
// prettier-ignore
function FromSchema(type, propertyKeys) {
    return ((0, index_5.UnionEvaluated)(IndexFromPropertyKeys(type, propertyKeys)));
}
// prettier-ignore
function IndexFromComputed(type, key) {
    return (0, index_2.Computed)('Index', [type, key]);
}
/** `[Json]` Returns an Indexed property type for the given keys */
function Index(type, key, options) {
    // computed-type
    if ((0, kind_1.IsRef)(type) || (0, kind_1.IsRef)(key)) {
        const error = `Index types using Ref parameters require both Type and Key to be of TSchema`;
        if (!(0, kind_1.IsSchema)(type) || !(0, kind_1.IsSchema)(key))
            throw new index_1.TypeBoxError(error);
        return (0, index_2.Computed)('Index', [type, key]);
    }
    // mapped-types
    if ((0, kind_1.IsMappedResult)(key))
        return (0, indexed_from_mapped_result_1.IndexFromMappedResult)(type, key, options);
    if ((0, kind_1.IsMappedKey)(key))
        return (0, indexed_from_mapped_key_1.IndexFromMappedKey)(type, key, options);
    // prettier-ignore
    return (0, type_1.CreateType)((0, kind_1.IsSchema)(key)
        ? FromSchema(type, (0, indexed_property_keys_1.IndexPropertyKeys)(key))
        : FromSchema(type, key), options);
}
