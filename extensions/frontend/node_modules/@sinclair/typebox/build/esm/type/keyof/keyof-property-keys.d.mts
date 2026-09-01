import type { TSchema } from '../schema/index.mjs';
import { type ZeroString, type UnionToTuple, type TIncrement } from '../helpers/index.mjs';
import type { TRecursive } from '../recursive/index.mjs';
import type { TIntersect } from '../intersect/index.mjs';
import type { TUnion } from '../union/index.mjs';
import type { TTuple } from '../tuple/index.mjs';
import type { TArray } from '../array/index.mjs';
import type { TObject, TProperties } from '../object/index.mjs';
import { type TSetUnionMany, type TSetIntersectMany } from '../sets/index.mjs';
type TFromRest<Types extends TSchema[], Result extends PropertyKey[][] = []> = (Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromRest<R, [...Result, TKeyOfPropertyKeys<L>]> : Result);
type TFromIntersect<Types extends TSchema[], PropertyKeysArray extends PropertyKey[][] = TFromRest<Types>, PropertyKeys extends PropertyKey[] = TSetUnionMany<PropertyKeysArray>> = PropertyKeys;
type TFromUnion<Types extends TSchema[], PropertyKeysArray extends PropertyKey[][] = TFromRest<Types>, PropertyKeys extends PropertyKey[] = TSetIntersectMany<PropertyKeysArray>> = PropertyKeys;
type TFromTuple<Types extends TSchema[], Indexer extends string = ZeroString, Acc extends PropertyKey[] = []> = Types extends [infer _ extends TSchema, ...infer R extends TSchema[]] ? TFromTuple<R, TIncrement<Indexer>, [...Acc, Indexer]> : Acc;
type TFromArray<_ extends TSchema> = ([
    '[number]'
]);
type TFromProperties<Properties extends TProperties> = (UnionToTuple<keyof Properties>);
export type TKeyOfPropertyKeys<Type extends TSchema> = (Type extends TRecursive<infer Type extends TSchema> ? TKeyOfPropertyKeys<Type> : Type extends TIntersect<infer Types extends TSchema[]> ? TFromIntersect<Types> : Type extends TUnion<infer Types extends TSchema[]> ? TFromUnion<Types> : Type extends TTuple<infer Types extends TSchema[]> ? TFromTuple<Types> : Type extends TArray<infer Type extends TSchema> ? TFromArray<Type> : Type extends TObject<infer Properties extends TProperties> ? TFromProperties<Properties> : [
]);
/** Returns a tuple of PropertyKeys derived from the given TSchema. */
export declare function KeyOfPropertyKeys<Type extends TSchema>(type: Type): TKeyOfPropertyKeys<Type>;
/** Returns a regular expression pattern derived from the given TSchema */
export declare function KeyOfPattern(schema: TSchema): string;
export {};
