import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { TupleToUnion, Evaluate } from '../helpers/index.mjs';
import { type TRecursive } from '../recursive/index.mjs';
import { type TComputed } from '../computed/index.mjs';
import { type TIntersect } from '../intersect/index.mjs';
import { type TLiteral, type TLiteralValue } from '../literal/index.mjs';
import { type TObject, type TProperties } from '../object/index.mjs';
import { type TUnion } from '../union/index.mjs';
import { type TMappedKey, type TMappedResult } from '../mapped/index.mjs';
import { type TRef } from '../ref/index.mjs';
import { type TIndexPropertyKeys } from '../indexed/index.mjs';
import { type TPickFromMappedKey } from './pick-from-mapped-key.mjs';
import { type TPickFromMappedResult } from './pick-from-mapped-result.mjs';
type TFromIntersect<Types extends TSchema[], PropertyKeys extends PropertyKey[], Result extends TSchema[] = []> = Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromIntersect<R, PropertyKeys, [...Result, TPick<L, PropertyKeys>]> : Result;
type TFromUnion<Types extends TSchema[], PropertyKeys extends PropertyKey[], Result extends TSchema[] = []> = Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromUnion<R, PropertyKeys, [...Result, TPick<L, PropertyKeys>]> : Result;
type TFromProperties<Properties extends TProperties, PropertyKeys extends PropertyKey[], UnionKeys extends PropertyKey = TupleToUnion<PropertyKeys>> = (Evaluate<Pick<Properties, UnionKeys & keyof Properties>>);
type TFromObject<_Type extends TObject, Keys extends PropertyKey[], Properties extends TProperties, MappedProperties extends TProperties = TFromProperties<Properties, Keys>, Result extends TSchema = TObject<MappedProperties>> = Result;
type TUnionFromPropertyKeys<PropertyKeys extends PropertyKey[], Result extends TLiteral[] = []> = (PropertyKeys extends [infer Key extends PropertyKey, ...infer Rest extends PropertyKey[]] ? Key extends TLiteralValue ? TUnionFromPropertyKeys<Rest, [...Result, TLiteral<Key>]> : TUnionFromPropertyKeys<Rest, [...Result]> : TUnion<Result>);
export type TPickResolve<Type extends TProperties, PropertyKeys extends PropertyKey[]> = (Type extends TRecursive<infer Types extends TSchema> ? TRecursive<TPickResolve<Types, PropertyKeys>> : Type extends TIntersect<infer Types extends TSchema[]> ? TIntersect<TFromIntersect<Types, PropertyKeys>> : Type extends TUnion<infer Types extends TSchema[]> ? TUnion<TFromUnion<Types, PropertyKeys>> : Type extends TObject<infer Properties extends TProperties> ? TFromObject<TObject, PropertyKeys, Properties> : TObject<{}>);
type TResolvePropertyKeys<Key extends TSchema | PropertyKey[]> = Key extends TSchema ? TIndexPropertyKeys<Key> : Key;
type TResolveTypeKey<Key extends TSchema | PropertyKey[]> = Key extends PropertyKey[] ? TUnionFromPropertyKeys<Key> : Key;
export type TPick<Type extends TSchema, Key extends TSchema | PropertyKey[], IsTypeRef extends boolean = Type extends TRef ? true : false, IsKeyRef extends boolean = Key extends TRef ? true : false> = (Type extends TMappedResult ? TPickFromMappedResult<Type, TResolvePropertyKeys<Key>> : Key extends TMappedKey ? TPickFromMappedKey<Type, Key> : [
    IsTypeRef,
    IsKeyRef
] extends [true, true] ? TComputed<'Pick', [Type, TResolveTypeKey<Key>]> : [
    IsTypeRef,
    IsKeyRef
] extends [false, true] ? TComputed<'Pick', [Type, TResolveTypeKey<Key>]> : [
    IsTypeRef,
    IsKeyRef
] extends [true, false] ? TComputed<'Pick', [Type, TResolveTypeKey<Key>]> : TPickResolve<Type, TResolvePropertyKeys<Key>>);
/** `[Json]` Constructs a type whose keys are picked from the given type */
export declare function Pick<Type extends TSchema, Key extends PropertyKey[]>(type: Type, key: readonly [...Key], options?: SchemaOptions): TPick<Type, Key>;
/** `[Json]` Constructs a type whose keys are picked from the given type */
export declare function Pick<Type extends TSchema, Key extends TSchema>(type: Type, key: Key, options?: SchemaOptions): TPick<Type, Key>;
export {};
