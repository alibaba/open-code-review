import { type TSchema, SchemaOptions } from '../schema/index';
import { type Assert } from '../helpers/index';
import { type TComputed } from '../computed/index';
import { type TNever } from '../never/index';
import { type TArray } from '../array/index';
import { type TIntersect } from '../intersect/index';
import { type TMappedResult, type TMappedKey } from '../mapped/index';
import { type TObject, type TProperties } from '../object/index';
import { type TUnion } from '../union/index';
import { type TRecursive } from '../recursive/index';
import { type TRef } from '../ref/index';
import { type TTuple } from '../tuple/index';
import { type TIntersectEvaluated } from '../intersect/index';
import { type TUnionEvaluated } from '../union/index';
import { type TIndexPropertyKeys } from './indexed-property-keys';
import { type TIndexFromMappedKey } from './indexed-from-mapped-key';
import { type TIndexFromMappedResult } from './indexed-from-mapped-result';
type TFromRest<Types extends TSchema[], Key extends PropertyKey, Result extends TSchema[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? TFromRest<Right, Key, [...Result, Assert<TIndexFromPropertyKey<Left, Key>, TSchema>]> : Result);
type TFromIntersectRest<Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? Left extends TNever ? TFromIntersectRest<Right, [...Result]> : TFromIntersectRest<Right, [...Result, Left]> : Result);
type TFromIntersect<Types extends TSchema[], Key extends PropertyKey> = (TIntersectEvaluated<TFromIntersectRest<TFromRest<Types, Key>>>);
type TFromUnionRest<Types extends TSchema[], Result extends TSchema[] = []> = Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? Left extends TNever ? [] : TFromUnionRest<Right, [Left, ...Result]> : Result;
type TFromUnion<Types extends TSchema[], Key extends PropertyKey> = (TUnionEvaluated<TFromUnionRest<TFromRest<Types, Key>>>);
type TFromTuple<Types extends TSchema[], Key extends PropertyKey> = (Key extends keyof Types ? Types[Key] : Key extends '[number]' ? TUnionEvaluated<Types> : TNever);
type TFromArray<Type extends TSchema, Key extends PropertyKey> = (Key extends '[number]' ? Type : TNever);
type AssertPropertyKey<T> = Assert<T, string | number>;
type TFromProperty<Properties extends TProperties, Key extends PropertyKey> = (Key extends keyof Properties ? Properties[Key] : `${AssertPropertyKey<Key>}` extends `${AssertPropertyKey<keyof Properties>}` ? Properties[AssertPropertyKey<Key>] : TNever);
export type TIndexFromPropertyKey<Type extends TSchema, Key extends PropertyKey> = (Type extends TRecursive<infer Type extends TSchema> ? TIndexFromPropertyKey<Type, Key> : Type extends TIntersect<infer Types extends TSchema[]> ? TFromIntersect<Types, Key> : Type extends TUnion<infer Types extends TSchema[]> ? TFromUnion<Types, Key> : Type extends TTuple<infer Types extends TSchema[]> ? TFromTuple<Types, Key> : Type extends TArray<infer Type extends TSchema> ? TFromArray<Type, Key> : Type extends TObject<infer Properties extends TProperties> ? TFromProperty<Properties, Key> : TNever);
export declare function IndexFromPropertyKey<Type extends TSchema, Key extends PropertyKey>(type: Type, propertyKey: Key): TIndexFromPropertyKey<Type, Key>;
export type TIndexFromPropertyKeys<Type extends TSchema, PropertyKeys extends PropertyKey[], Result extends TSchema[] = []> = (PropertyKeys extends [infer Left extends PropertyKey, ...infer Right extends PropertyKey[]] ? TIndexFromPropertyKeys<Type, Right, [...Result, Assert<TIndexFromPropertyKey<Type, Left>, TSchema>]> : Result);
export declare function IndexFromPropertyKeys<Type extends TSchema, PropertyKeys extends PropertyKey[]>(type: Type, propertyKeys: [...PropertyKeys]): TIndexFromPropertyKeys<Type, PropertyKeys>;
type FromSchema<Type extends TSchema, PropertyKeys extends PropertyKey[]> = (TUnionEvaluated<TIndexFromPropertyKeys<Type, PropertyKeys>>);
declare function FromSchema<Type extends TSchema, PropertyKeys extends PropertyKey[]>(type: Type, propertyKeys: [...PropertyKeys]): FromSchema<Type, PropertyKeys>;
export type TIndexFromComputed<Type extends TSchema, Key extends TSchema> = (TComputed<'Index', [Type, Key]>);
export declare function IndexFromComputed<Type extends TSchema, Key extends TSchema>(type: Type, key: Key): TIndexFromComputed<Type, Key>;
export type TIndex<Type extends TSchema, PropertyKeys extends PropertyKey[]> = (FromSchema<Type, PropertyKeys>);
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TRef, Key extends TSchema>(type: Type, key: Key, options?: SchemaOptions): TIndexFromComputed<Type, Key>;
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TSchema, Key extends TRef>(type: Type, key: Key, options?: SchemaOptions): TIndexFromComputed<Type, Key>;
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TRef, Key extends TRef>(type: Type, key: Key, options?: SchemaOptions): TIndexFromComputed<Type, Key>;
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TSchema, MappedResult extends TMappedResult>(type: Type, mappedResult: MappedResult, options?: SchemaOptions): TIndexFromMappedResult<Type, MappedResult>;
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TSchema, MappedResult extends TMappedResult>(type: Type, mappedResult: MappedResult, options?: SchemaOptions): TIndexFromMappedResult<Type, MappedResult>;
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TSchema, MappedKey extends TMappedKey>(type: Type, mappedKey: MappedKey, options?: SchemaOptions): TIndexFromMappedKey<Type, MappedKey>;
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TSchema, Key extends TSchema, PropertyKeys extends PropertyKey[] = TIndexPropertyKeys<Key>>(T: Type, K: Key, options?: SchemaOptions): TIndex<Type, PropertyKeys>;
/** `[Json]` Returns an Indexed property type for the given keys */
export declare function Index<Type extends TSchema, PropertyKeys extends PropertyKey[]>(type: Type, propertyKeys: readonly [...PropertyKeys], options?: SchemaOptions): TIndex<Type, PropertyKeys>;
export {};
