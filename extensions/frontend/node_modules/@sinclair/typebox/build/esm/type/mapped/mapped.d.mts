import type { TSchema } from '../schema/index.mjs';
import type { Ensure, Evaluate, Assert } from '../helpers/index.mjs';
import { type TArray } from '../array/index.mjs';
import { type TAsyncIterator } from '../async-iterator/index.mjs';
import { type TConstructor } from '../constructor/index.mjs';
import { type TEnum, type TEnumRecord } from '../enum/index.mjs';
import { type TFunction } from '../function/index.mjs';
import { type TIndexPropertyKeys } from '../indexed/index.mjs';
import { type TIntersect } from '../intersect/index.mjs';
import { type TIterator } from '../iterator/index.mjs';
import { type TLiteral, type TLiteralValue } from '../literal/index.mjs';
import { type TObject, type TProperties, type ObjectOptions } from '../object/index.mjs';
import { type TOptional } from '../optional/index.mjs';
import { type TPromise } from '../promise/index.mjs';
import { type TReadonly } from '../readonly/index.mjs';
import { type TTuple } from '../tuple/index.mjs';
import { type TUnion } from '../union/index.mjs';
import { type TSetIncludes } from '../sets/index.mjs';
import { type TMappedResult } from './mapped-result.mjs';
import type { TMappedKey } from './mapped-key.mjs';
type TFromMappedResult<K extends PropertyKey, P extends TProperties> = (K extends keyof P ? FromSchemaType<K, P[K]> : TMappedResult<P>);
type TMappedKeyToKnownMappedResultProperties<K extends PropertyKey> = {
    [_ in K]: TLiteral<Assert<K, TLiteralValue>>;
};
type TMappedKeyToUnknownMappedResultProperties<P extends PropertyKey[], Acc extends TProperties = {}> = (P extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TMappedKeyToUnknownMappedResultProperties<R, Acc & {
    [_ in L]: TLiteral<Assert<L, TLiteralValue>>;
}> : Acc);
type TMappedKeyToMappedResultProperties<K extends PropertyKey, P extends PropertyKey[]> = (TSetIncludes<P, K> extends true ? TMappedKeyToKnownMappedResultProperties<K> : TMappedKeyToUnknownMappedResultProperties<P>);
type TFromMappedKey<K extends PropertyKey, P extends PropertyKey[], R extends TProperties = TMappedKeyToMappedResultProperties<K, P>> = (TFromMappedResult<K, R>);
type TFromRest<K extends PropertyKey, T extends TSchema[], Acc extends TSchema[] = []> = (T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromRest<K, R, [...Acc, FromSchemaType<K, L>]> : Acc);
type FromProperties<K extends PropertyKey, T extends TProperties, R extends TProperties = Evaluate<{
    [K2 in keyof T]: FromSchemaType<K, T[K2]>;
}>> = R;
declare function FromProperties<K extends PropertyKey, T extends TProperties>(K: K, T: T): FromProperties<K, T>;
type FromSchemaType<K extends PropertyKey, T extends TSchema> = (T extends TReadonly<infer S extends TSchema> ? TReadonly<FromSchemaType<K, S>> : T extends TOptional<infer S extends TSchema> ? TOptional<FromSchemaType<K, S>> : T extends TMappedResult<infer P extends TProperties> ? TFromMappedResult<K, P> : T extends TMappedKey<infer P extends PropertyKey[]> ? TFromMappedKey<K, P> : T extends TConstructor<infer S extends TSchema[], infer R extends TSchema> ? TConstructor<TFromRest<K, S>, FromSchemaType<K, R>> : T extends TFunction<infer S extends TSchema[], infer R extends TSchema> ? TFunction<TFromRest<K, S>, FromSchemaType<K, R>> : T extends TAsyncIterator<infer S extends TSchema> ? TAsyncIterator<FromSchemaType<K, S>> : T extends TIterator<infer S extends TSchema> ? TIterator<FromSchemaType<K, S>> : T extends TIntersect<infer S extends TSchema[]> ? TIntersect<TFromRest<K, S>> : T extends TEnum<infer S extends TEnumRecord> ? TEnum<S> : T extends TUnion<infer S extends TSchema[]> ? TUnion<TFromRest<K, S>> : T extends TTuple<infer S extends TSchema[]> ? TTuple<TFromRest<K, S>> : T extends TObject<infer S extends TProperties> ? TObject<FromProperties<K, S>> : T extends TArray<infer S extends TSchema> ? TArray<FromSchemaType<K, S>> : T extends TPromise<infer S extends TSchema> ? TPromise<FromSchemaType<K, S>> : T);
declare function FromSchemaType<K extends PropertyKey, T extends TSchema>(K: K, T: T): FromSchemaType<K, T>;
export type TMappedFunctionReturnType<K extends PropertyKey[], T extends TSchema, Acc extends TProperties = {}> = (K extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TMappedFunctionReturnType<R, T, Acc & {
    [_ in L]: FromSchemaType<L, T>;
}> : Acc);
export declare function MappedFunctionReturnType<K extends PropertyKey[], T extends TSchema>(K: [...K], T: T): TMappedFunctionReturnType<K, T>;
export type TMappedFunction<K extends PropertyKey[], I = TMappedKey<K>> = (T: I) => TSchema;
export type TMapped<K extends PropertyKey[], F extends TMappedFunction<K>, R extends TProperties = Evaluate<TMappedFunctionReturnType<K, ReturnType<F>>>> = Ensure<TObject<R>>;
/** `[Json]` Creates a Mapped object type */
export declare function Mapped<K extends TSchema, I extends PropertyKey[] = TIndexPropertyKeys<K>, F extends TMappedFunction<I> = TMappedFunction<I>, R extends TMapped<I, F> = TMapped<I, F>>(key: K, map: F, options?: ObjectOptions): R;
/** `[Json]` Creates a Mapped object type */
export declare function Mapped<K extends PropertyKey[], F extends TMappedFunction<K> = TMappedFunction<K>, R extends TMapped<K, F> = TMapped<K, F>>(key: [...K], map: F, options?: ObjectOptions): R;
export {};
