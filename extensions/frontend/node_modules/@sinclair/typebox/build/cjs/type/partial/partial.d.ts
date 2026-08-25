import type { TSchema, SchemaOptions } from '../schema/index';
import type { Evaluate, Ensure } from '../helpers/index';
import type { TMappedResult } from '../mapped/index';
import { type TReadonlyOptional } from '../readonly-optional/index';
import { type TComputed } from '../computed/index';
import { type TOptional } from '../optional/index';
import { type TReadonly } from '../readonly/index';
import { type TRecursive } from '../recursive/index';
import { type TObject, type TProperties } from '../object/index';
import { type TIntersect } from '../intersect/index';
import { type TUnion } from '../union/index';
import { type TRef } from '../ref/index';
import { type TBigInt } from '../bigint/index';
import { type TBoolean } from '../boolean/index';
import { type TInteger } from '../integer/index';
import { type TLiteral } from '../literal/index';
import { type TNull } from '../null/index';
import { type TNumber } from '../number/index';
import { type TString } from '../string/index';
import { type TSymbol } from '../symbol/index';
import { type TUndefined } from '../undefined/index';
import { type TPartialFromMappedResult } from './partial-from-mapped-result';
type TFromComputed<Target extends string, Parameters extends TSchema[]> = Ensure<TComputed<'Partial', [TComputed<Target, Parameters>]>>;
type TFromRef<Ref extends string> = Ensure<TComputed<'Partial', [TRef<Ref>]>>;
type TFromProperties<Properties extends TProperties> = Evaluate<{
    [K in keyof Properties]: Properties[K] extends (TReadonlyOptional<infer S>) ? TReadonlyOptional<S> : Properties[K] extends (TReadonly<infer S>) ? TReadonlyOptional<S> : Properties[K] extends (TOptional<infer S>) ? TOptional<S> : TOptional<Properties[K]>;
}>;
type TFromObject<_Type extends TObject, Properties extends TProperties, MappedProperties extends TProperties = TFromProperties<Properties>, Result extends TSchema = TObject<MappedProperties>> = Result;
type TFromRest<Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromRest<R, [...Result, TPartial<L>]> : Result);
export type TPartial<Type extends TSchema> = (Type extends TRecursive<infer Type extends TSchema> ? TRecursive<TPartial<Type>> : Type extends TComputed<infer Target extends string, infer Parameters extends TSchema[]> ? TFromComputed<Target, Parameters> : Type extends TRef<infer Ref extends string> ? TFromRef<Ref> : Type extends TIntersect<infer Types extends TSchema[]> ? TIntersect<TFromRest<Types>> : Type extends TUnion<infer Types extends TSchema[]> ? TUnion<TFromRest<Types>> : Type extends TObject<infer Properties extends TProperties> ? TFromObject<TObject, Properties> : Type extends TBigInt ? Type : Type extends TBoolean ? Type : Type extends TInteger ? Type : Type extends TLiteral ? Type : Type extends TNull ? Type : Type extends TNumber ? Type : Type extends TString ? Type : Type extends TSymbol ? Type : Type extends TUndefined ? Type : TObject<{}>);
/** `[Json]` Constructs a type where all properties are optional */
export declare function Partial<MappedResult extends TMappedResult>(type: MappedResult, options?: SchemaOptions): TPartialFromMappedResult<MappedResult>;
/** `[Json]` Constructs a type where all properties are optional */
export declare function Partial<Type extends TSchema>(type: Type, options?: SchemaOptions): TPartial<Type>;
export {};
