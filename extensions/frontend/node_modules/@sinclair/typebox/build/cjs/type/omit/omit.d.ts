import type { SchemaOptions, TSchema } from '../schema/index';
import type { TupleToUnion, Evaluate } from '../helpers/index';
import { type TRecursive } from '../recursive/index';
import type { TMappedKey, TMappedResult } from '../mapped/index';
import { TComputed } from '../computed/index';
import { TLiteral, TLiteralValue } from '../literal/index';
import { type TIndexPropertyKeys } from '../indexed/index';
import { type TIntersect } from '../intersect/index';
import { type TUnion } from '../union/index';
import { type TObject, type TProperties } from '../object/index';
import { type TRef } from '../ref/index';
import { type TOmitFromMappedKey } from './omit-from-mapped-key';
import { type TOmitFromMappedResult } from './omit-from-mapped-result';
type TFromIntersect<Types extends TSchema[], PropertyKeys extends PropertyKey[], Result extends TSchema[] = []> = (Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromIntersect<R, PropertyKeys, [...Result, TOmit<L, PropertyKeys>]> : Result);
type TFromUnion<T extends TSchema[], K extends PropertyKey[], Result extends TSchema[] = []> = (T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromUnion<R, K, [...Result, TOmit<L, K>]> : Result);
type TFromProperties<Properties extends TProperties, PropertyKeys extends PropertyKey[], UnionKey extends PropertyKey = TupleToUnion<PropertyKeys>> = (Evaluate<Omit<Properties, UnionKey>>);
type TFromObject<_Type extends TObject, PropertyKeys extends PropertyKey[], Properties extends TProperties, MappedProperties extends TProperties = TFromProperties<Properties, PropertyKeys>, Result extends TSchema = TObject<MappedProperties>> = Result;
type TUnionFromPropertyKeys<PropertyKeys extends PropertyKey[], Result extends TLiteral[] = []> = (PropertyKeys extends [infer Key extends PropertyKey, ...infer Rest extends PropertyKey[]] ? Key extends TLiteralValue ? TUnionFromPropertyKeys<Rest, [...Result, TLiteral<Key>]> : TUnionFromPropertyKeys<Rest, [...Result]> : TUnion<Result>);
export type TOmitResolve<Properties extends TProperties, PropertyKeys extends PropertyKey[]> = (Properties extends TRecursive<infer Types extends TSchema> ? TRecursive<TOmitResolve<Types, PropertyKeys>> : Properties extends TIntersect<infer Types extends TSchema[]> ? TIntersect<TFromIntersect<Types, PropertyKeys>> : Properties extends TUnion<infer Types extends TSchema[]> ? TUnion<TFromUnion<Types, PropertyKeys>> : Properties extends TObject<infer Properties extends TProperties> ? TFromObject<TObject, PropertyKeys, Properties> : TObject<{}>);
type TResolvePropertyKeys<Key extends TSchema | PropertyKey[]> = Key extends TSchema ? TIndexPropertyKeys<Key> : Key;
type TResolveTypeKey<Key extends TSchema | PropertyKey[]> = Key extends PropertyKey[] ? TUnionFromPropertyKeys<Key> : Key;
export type TOmit<Type extends TSchema, Key extends TSchema | PropertyKey[], IsTypeRef extends boolean = Type extends TRef ? true : false, IsKeyRef extends boolean = Key extends TRef ? true : false> = (Type extends TMappedResult ? TOmitFromMappedResult<Type, TResolvePropertyKeys<Key>> : Key extends TMappedKey ? TOmitFromMappedKey<Type, Key> : [
    IsTypeRef,
    IsKeyRef
] extends [true, true] ? TComputed<'Omit', [Type, TResolveTypeKey<Key>]> : [
    IsTypeRef,
    IsKeyRef
] extends [false, true] ? TComputed<'Omit', [Type, TResolveTypeKey<Key>]> : [
    IsTypeRef,
    IsKeyRef
] extends [true, false] ? TComputed<'Omit', [Type, TResolveTypeKey<Key>]> : TOmitResolve<Type, TResolvePropertyKeys<Key>>);
/** `[Json]` Constructs a type whose keys are picked from the given type */
export declare function Omit<Type extends TSchema, Key extends PropertyKey[]>(type: Type, key: readonly [...Key], options?: SchemaOptions): TOmit<Type, Key>;
/** `[Json]` Constructs a type whose keys are picked from the given type */
export declare function Omit<Type extends TSchema, Key extends TSchema>(type: Type, key: Key, options?: SchemaOptions): TOmit<Type, Key>;
export {};
