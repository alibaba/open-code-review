import { CreateType } from '../create/type.mjs';
import { Discard } from '../discard/discard.mjs';
import { Computed } from '../computed/index.mjs';
import { Intersect } from '../intersect/index.mjs';
import { Literal } from '../literal/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { Union } from '../union/index.mjs';
import { IndexPropertyKeys } from '../indexed/index.mjs';
import { TransformKind } from '../symbols/symbols.mjs';
// ------------------------------------------------------------------
// Guards
// ------------------------------------------------------------------
import { IsMappedKey, IsMappedResult, IsIntersect, IsUnion, IsObject, IsSchema, IsLiteralValue, IsRef } from '../guard/kind.mjs';
import { IsArray as IsArrayValue } from '../guard/value.mjs';
// ------------------------------------------------------------------
// Infrastructure
// ------------------------------------------------------------------
import { PickFromMappedKey } from './pick-from-mapped-key.mjs';
import { PickFromMappedResult } from './pick-from-mapped-result.mjs';
function FromIntersect(types, propertyKeys) {
    return types.map((type) => PickResolve(type, propertyKeys));
}
// prettier-ignore
function FromUnion(types, propertyKeys) {
    return types.map((type) => PickResolve(type, propertyKeys));
}
// prettier-ignore
function FromProperties(properties, propertyKeys) {
    const result = {};
    for (const K2 of propertyKeys)
        if (K2 in properties)
            result[K2] = properties[K2];
    return result;
}
// prettier-ignore
function FromObject(Type, keys, properties) {
    const options = Discard(Type, [TransformKind, '$id', 'required', 'properties']);
    const mappedProperties = FromProperties(properties, keys);
    return _Object_(mappedProperties, options);
}
// prettier-ignore
function UnionFromPropertyKeys(propertyKeys) {
    const result = propertyKeys.reduce((result, key) => IsLiteralValue(key) ? [...result, Literal(key)] : result, []);
    return Union(result);
}
// prettier-ignore
function PickResolve(type, propertyKeys) {
    return (IsIntersect(type) ? Intersect(FromIntersect(type.allOf, propertyKeys)) :
        IsUnion(type) ? Union(FromUnion(type.anyOf, propertyKeys)) :
            IsObject(type) ? FromObject(type, propertyKeys, type.properties) :
                _Object_({}));
}
/** `[Json]` Constructs a type whose keys are picked from the given type */
// prettier-ignore
export function Pick(type, key, options) {
    const typeKey = IsArrayValue(key) ? UnionFromPropertyKeys(key) : key;
    const propertyKeys = IsSchema(key) ? IndexPropertyKeys(key) : key;
    const isTypeRef = IsRef(type);
    const isKeyRef = IsRef(key);
    return (IsMappedResult(type) ? PickFromMappedResult(type, propertyKeys, options) :
        IsMappedKey(key) ? PickFromMappedKey(type, key, options) :
            (isTypeRef && isKeyRef) ? Computed('Pick', [type, typeKey], options) :
                (!isTypeRef && isKeyRef) ? Computed('Pick', [type, typeKey], options) :
                    (isTypeRef && !isKeyRef) ? Computed('Pick', [type, typeKey], options) :
                        CreateType({ ...PickResolve(type, propertyKeys), ...options }));
}
