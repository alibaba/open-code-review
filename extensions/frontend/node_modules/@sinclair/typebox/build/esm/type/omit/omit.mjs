import { CreateType } from '../create/type.mjs';
import { Discard } from '../discard/discard.mjs';
import { TransformKind } from '../symbols/symbols.mjs';
import { Computed } from '../computed/index.mjs';
import { Literal } from '../literal/index.mjs';
import { IndexPropertyKeys } from '../indexed/index.mjs';
import { Intersect } from '../intersect/index.mjs';
import { Union } from '../union/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
// ------------------------------------------------------------------
// Mapped
// ------------------------------------------------------------------
import { OmitFromMappedKey } from './omit-from-mapped-key.mjs';
import { OmitFromMappedResult } from './omit-from-mapped-result.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsMappedKey, IsIntersect, IsUnion, IsObject, IsSchema, IsMappedResult, IsLiteralValue, IsRef } from '../guard/kind.mjs';
import { IsArray as IsArrayValue } from '../guard/value.mjs';
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
    const options = Discard(type, [TransformKind, '$id', 'required', 'properties']);
    const mappedProperties = FromProperties(properties, propertyKeys);
    return _Object_(mappedProperties, options);
}
// prettier-ignore
function UnionFromPropertyKeys(propertyKeys) {
    const result = propertyKeys.reduce((result, key) => IsLiteralValue(key) ? [...result, Literal(key)] : result, []);
    return Union(result);
}
// prettier-ignore
function OmitResolve(type, propertyKeys) {
    return (IsIntersect(type) ? Intersect(FromIntersect(type.allOf, propertyKeys)) :
        IsUnion(type) ? Union(FromUnion(type.anyOf, propertyKeys)) :
            IsObject(type) ? FromObject(type, propertyKeys, type.properties) :
                _Object_({}));
}
/** `[Json]` Constructs a type whose keys are picked from the given type */
// prettier-ignore
export function Omit(type, key, options) {
    const typeKey = IsArrayValue(key) ? UnionFromPropertyKeys(key) : key;
    const propertyKeys = IsSchema(key) ? IndexPropertyKeys(key) : key;
    const isTypeRef = IsRef(type);
    const isKeyRef = IsRef(key);
    return (IsMappedResult(type) ? OmitFromMappedResult(type, propertyKeys, options) :
        IsMappedKey(key) ? OmitFromMappedKey(type, key, options) :
            (isTypeRef && isKeyRef) ? Computed('Omit', [type, typeKey], options) :
                (!isTypeRef && isKeyRef) ? Computed('Omit', [type, typeKey], options) :
                    (isTypeRef && !isKeyRef) ? Computed('Omit', [type, typeKey], options) :
                        CreateType({ ...OmitResolve(type, propertyKeys), ...options }));
}
