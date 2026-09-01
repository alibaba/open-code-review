import { Kind, OptionalKind, ReadonlyKind } from '../symbols/index.mjs';
import { Discard } from '../discard/index.mjs';
// evaluation types
import { Array } from '../array/index.mjs';
import { AsyncIterator } from '../async-iterator/index.mjs';
import { Constructor } from '../constructor/index.mjs';
import { Function as FunctionType } from '../function/index.mjs';
import { IndexPropertyKeys } from '../indexed/index.mjs';
import { Intersect } from '../intersect/index.mjs';
import { Iterator } from '../iterator/index.mjs';
import { Literal } from '../literal/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { Optional } from '../optional/index.mjs';
import { Promise } from '../promise/index.mjs';
import { Readonly } from '../readonly/index.mjs';
import { Tuple } from '../tuple/index.mjs';
import { Union } from '../union/index.mjs';
// operator
import { SetIncludes } from '../sets/index.mjs';
// mapping types
import { MappedResult } from './mapped-result.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsArray, IsAsyncIterator, IsConstructor, IsFunction, IsIntersect, IsIterator, IsReadonly, IsMappedResult, IsMappedKey, IsObject, IsOptional, IsPromise, IsSchema, IsTuple, IsUnion } from '../guard/kind.mjs';
// prettier-ignore
function FromMappedResult(K, P) {
    return (K in P
        ? FromSchemaType(K, P[K])
        : MappedResult(P));
}
// prettier-ignore
function MappedKeyToKnownMappedResultProperties(K) {
    return { [K]: Literal(K) };
}
// prettier-ignore
function MappedKeyToUnknownMappedResultProperties(P) {
    const Acc = {};
    for (const L of P)
        Acc[L] = Literal(L);
    return Acc;
}
// prettier-ignore
function MappedKeyToMappedResultProperties(K, P) {
    return (SetIncludes(P, K)
        ? MappedKeyToKnownMappedResultProperties(K)
        : MappedKeyToUnknownMappedResultProperties(P));
}
// prettier-ignore
function FromMappedKey(K, P) {
    const R = MappedKeyToMappedResultProperties(K, P);
    return FromMappedResult(K, R);
}
// prettier-ignore
function FromRest(K, T) {
    return T.map(L => FromSchemaType(K, L));
}
// prettier-ignore
function FromProperties(K, T) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(T))
        Acc[K2] = FromSchemaType(K, T[K2]);
    return Acc;
}
// prettier-ignore
function FromSchemaType(K, T) {
    // required to retain user defined options for mapped type
    const options = { ...T };
    return (
    // unevaluated modifier types
    IsOptional(T) ? Optional(FromSchemaType(K, Discard(T, [OptionalKind]))) :
        IsReadonly(T) ? Readonly(FromSchemaType(K, Discard(T, [ReadonlyKind]))) :
            // unevaluated mapped types
            IsMappedResult(T) ? FromMappedResult(K, T.properties) :
                IsMappedKey(T) ? FromMappedKey(K, T.keys) :
                    // unevaluated types
                    IsConstructor(T) ? Constructor(FromRest(K, T.parameters), FromSchemaType(K, T.returns), options) :
                        IsFunction(T) ? FunctionType(FromRest(K, T.parameters), FromSchemaType(K, T.returns), options) :
                            IsAsyncIterator(T) ? AsyncIterator(FromSchemaType(K, T.items), options) :
                                IsIterator(T) ? Iterator(FromSchemaType(K, T.items), options) :
                                    IsIntersect(T) ? Intersect(FromRest(K, T.allOf), options) :
                                        IsUnion(T) ? Union(FromRest(K, T.anyOf), options) :
                                            IsTuple(T) ? Tuple(FromRest(K, T.items ?? []), options) :
                                                IsObject(T) ? _Object_(FromProperties(K, T.properties), options) :
                                                    IsArray(T) ? Array(FromSchemaType(K, T.items), options) :
                                                        IsPromise(T) ? Promise(FromSchemaType(K, T.item), options) :
                                                            T);
}
// prettier-ignore
export function MappedFunctionReturnType(K, T) {
    const Acc = {};
    for (const L of K)
        Acc[L] = FromSchemaType(L, T);
    return Acc;
}
/** `[Json]` Creates a Mapped object type */
export function Mapped(key, map, options) {
    const K = IsSchema(key) ? IndexPropertyKeys(key) : key;
    const RT = map({ [Kind]: 'MappedKey', keys: K });
    const R = MappedFunctionReturnType(K, RT);
    return _Object_(R, options);
}
