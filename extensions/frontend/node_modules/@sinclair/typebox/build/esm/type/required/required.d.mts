import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { Evaluate, Ensure } from '../helpers/index.mjs';
import type { TMappedResult } from '../mapped/index.mjs';
import { type TReadonlyOptional } from '../readonly-optional/index.mjs';
import { type TComputed } from '../computed/index.mjs';
import { type TOptional } from '../optional/index.mjs';
import { type TReadonly } from '../readonly/index.mjs';
import { type TRecursive } from '../recursive/index.mjs';
import { type TObject, type TProperties } from '../object/index.mjs';
import { type TIntersect } from '../intersect/index.mjs';
import { type TUnion } from '../union/index.mjs';
import { type TRef } from '../ref/index.mjs';
import { type TBigInt } from '../bigint/index.mjs';
import { type TBoolean } from '../boolean/index.mjs';
import { type TInteger } from '../integer/index.mjs';
import { type TLiteral } from '../literal/index.mjs';
import { type TNull } from '../null/index.mjs';
import { type TNumber } from '../number/index.mjs';
import { type TString } from '../string/index.mjs';
import { type TSymbol } from '../symbol/index.mjs';
import { type TUndefined } from '../undefined/index.mjs';
import { type TRequiredFromMappedResult } from './required-from-mapped-result.mjs';
type TFromComputed<Target extends string, Parameters extends TSchema[]> = Ensure<TComputed<'Required', [TComputed<Target, Parameters>]>>;
type TFromRef<Ref extends string> = Ensure<TComputed<'Required', [TRef<Ref>]>>;
type TFromProperties<Properties extends TProperties> = Evaluate<{
    [K in keyof Properties]: Properties[K] extends (TReadonlyOptional<infer S>) ? TReadonly<S> : Properties[K] extends (TReadonly<infer S>) ? TReadonly<S> : Properties[K] extends (TOptional<infer S>) ? S : Properties[K];
}>;
type TFromObject<_Type extends TObject, Properties extends TProperties, MappedProperties extends TProperties = TFromProperties<Properties>, Result extends TSchema = TObject<MappedProperties>> = Result;
type TFromRest<Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromRest<R, [...Result, TRequired<L>]> : Result);
export type TRequired<Type extends TSchema> = (Type extends TRecursive<infer Type extends TSchema> ? TRecursive<TRequired<Type>> : Type extends TComputed<infer Target extends string, infer Parameters extends TSchema[]> ? TFromComputed<Target, Parameters> : Type extends TRef<infer Ref extends string> ? TFromRef<Ref> : Type extends TIntersect<infer Types extends TSchema[]> ? TIntersect<TFromRest<Types>> : Type extends TUnion<infer Types extends TSchema[]> ? TUnion<TFromRest<Types>> : Type extends TObject<infer Properties extends TProperties> ? TFromObject<TObject, Properties> : Type extends TBigInt ? Type : Type extends TBoolean ? Type : Type extends TInteger ? Type : Type extends TLiteral ? Type : Type extends TNull ? Type : Type extends TNumber ? Type : Type extends TString ? Type : Type extends TSymbol ? Type : Type extends TUndefined ? Type : TObject<{}>);
/** `[Json]` Constructs a type where all properties are required */
export declare function Required<MappedResult extends TMappedResult>(type: MappedResult, options?: SchemaOptions): TRequiredFromMappedResult<MappedResult>;
/** `[Json]` Constructs a type where all properties are required */
export declare function Required<Type extends TSchema>(type: Type, options?: SchemaOptions): TRequired<Type>;
export {};
