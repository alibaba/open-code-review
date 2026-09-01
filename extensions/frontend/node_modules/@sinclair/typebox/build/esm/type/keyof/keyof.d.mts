import type { TSchema } from '../schema/index.mjs';
import type { Assert, Ensure } from '../helpers/index.mjs';
import type { TMappedResult } from '../mapped/index.mjs';
import type { SchemaOptions } from '../schema/index.mjs';
import { type TLiteral, type TLiteralValue } from '../literal/index.mjs';
import { type TNumber } from '../number/index.mjs';
import { TComputed } from '../computed/index.mjs';
import { type TRef } from '../ref/index.mjs';
import { type TKeyOfPropertyKeys } from './keyof-property-keys.mjs';
import { type TUnionEvaluated } from '../union/index.mjs';
import { type TKeyOfFromMappedResult } from './keyof-from-mapped-result.mjs';
type TFromComputed<Target extends string, Parameters extends TSchema[]> = Ensure<TComputed<'KeyOf', [TComputed<Target, Parameters>]>>;
type TFromRef<Ref extends string> = Ensure<TComputed<'KeyOf', [TRef<Ref>]>>;
/** `[Internal]` Used by KeyOfFromMappedResult */
export type TKeyOfFromType<Type extends TSchema, PropertyKeys extends PropertyKey[] = TKeyOfPropertyKeys<Type>, PropertyKeyTypes extends TSchema[] = TKeyOfPropertyKeysToRest<PropertyKeys>, Result = TUnionEvaluated<PropertyKeyTypes>> = Ensure<Result>;
export type TKeyOfPropertyKeysToRest<PropertyKeys extends PropertyKey[], Result extends TSchema[] = []> = (PropertyKeys extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? L extends '[number]' ? TKeyOfPropertyKeysToRest<R, [...Result, TNumber]> : TKeyOfPropertyKeysToRest<R, [...Result, TLiteral<Assert<L, TLiteralValue>>]> : Result);
export declare function KeyOfPropertyKeysToRest<PropertyKeys extends PropertyKey[]>(propertyKeys: [...PropertyKeys]): TKeyOfPropertyKeysToRest<PropertyKeys>;
export type TKeyOf<Type extends TSchema> = (Type extends TComputed<infer Target extends string, infer Parameters extends TSchema[]> ? TFromComputed<Target, Parameters> : Type extends TRef<infer Ref extends string> ? TFromRef<Ref> : Type extends TMappedResult ? TKeyOfFromMappedResult<Type> : TKeyOfFromType<Type>);
/** `[Json]` Creates a KeyOf type */
export declare function KeyOf<Type extends TSchema>(type: Type, options?: SchemaOptions): TKeyOf<Type>;
export {};
