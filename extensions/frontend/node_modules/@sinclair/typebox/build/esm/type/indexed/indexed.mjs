import { CreateType } from '../create/type.mjs';
import { TypeBoxError } from '../error/index.mjs';
import { Computed } from '../computed/index.mjs';
import { Never } from '../never/index.mjs';
import { IntersectEvaluated } from '../intersect/index.mjs';
import { UnionEvaluated } from '../union/index.mjs';
import { IndexPropertyKeys } from './indexed-property-keys.mjs';
import { IndexFromMappedKey } from './indexed-from-mapped-key.mjs';
import { IndexFromMappedResult } from './indexed-from-mapped-result.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsArray, IsIntersect, IsObject, IsMappedKey, IsMappedResult, IsNever, IsSchema, IsTuple, IsUnion, IsRef } from '../guard/kind.mjs';
// prettier-ignore
function FromRest(types, key) {
    return types.map(type => IndexFromPropertyKey(type, key));
}
// prettier-ignore
function FromIntersectRest(types) {
    return types.filter(type => !IsNever(type));
}
// prettier-ignore
function FromIntersect(types, key) {
    return (IntersectEvaluated(FromIntersectRest(FromRest(types, key))));
}
// prettier-ignore
function FromUnionRest(types) {
    return (types.some(L => IsNever(L))
        ? []
        : types);
}
// prettier-ignore
function FromUnion(types, key) {
    return (UnionEvaluated(FromUnionRest(FromRest(types, key))));
}
// prettier-ignore
function FromTuple(types, key) {
    return (key in types ? types[key] :
        key === '[number]' ? UnionEvaluated(types) :
            Never());
}
// prettier-ignore
function FromArray(type, key) {
    return (key === '[number]'
        ? type
        : Never());
}
// prettier-ignore
function FromProperty(properties, propertyKey) {
    return (propertyKey in properties ? properties[propertyKey] : Never());
}
// prettier-ignore
export function IndexFromPropertyKey(type, propertyKey) {
    return (IsIntersect(type) ? FromIntersect(type.allOf, propertyKey) :
        IsUnion(type) ? FromUnion(type.anyOf, propertyKey) :
            IsTuple(type) ? FromTuple(type.items ?? [], propertyKey) :
                IsArray(type) ? FromArray(type.items, propertyKey) :
                    IsObject(type) ? FromProperty(type.properties, propertyKey) :
                        Never());
}
// prettier-ignore
export function IndexFromPropertyKeys(type, propertyKeys) {
    return propertyKeys.map(propertyKey => IndexFromPropertyKey(type, propertyKey));
}
// prettier-ignore
function FromSchema(type, propertyKeys) {
    return (UnionEvaluated(IndexFromPropertyKeys(type, propertyKeys)));
}
// prettier-ignore
export function IndexFromComputed(type, key) {
    return Computed('Index', [type, key]);
}
/** `[Json]` Returns an Indexed property type for the given keys */
export function Index(type, key, options) {
    // computed-type
    if (IsRef(type) || IsRef(key)) {
        const error = `Index types using Ref parameters require both Type and Key to be of TSchema`;
        if (!IsSchema(type) || !IsSchema(key))
            throw new TypeBoxError(error);
        return Computed('Index', [type, key]);
    }
    // mapped-types
    if (IsMappedResult(key))
        return IndexFromMappedResult(type, key, options);
    if (IsMappedKey(key))
        return IndexFromMappedKey(type, key, options);
    // prettier-ignore
    return CreateType(IsSchema(key)
        ? FromSchema(type, IndexPropertyKeys(key))
        : FromSchema(type, key), options);
}
