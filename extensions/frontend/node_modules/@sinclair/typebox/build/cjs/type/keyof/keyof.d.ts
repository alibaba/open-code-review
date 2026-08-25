import type { TSchema } from '../schema/index';
import type { Assert, Ensure } from '../helpers/index';
import type { TMappedResult } from '../mapped/index';
import type { SchemaOptions } from '../schema/index';
import { type TLiteral, type TLiteralValue } from '../literal/index';
import { type TNumber } from '../number/index';
import { TComputed } from '../computed/index';
import { type TRef } from '../ref/index';
import { type TKeyOfPropertyKeys } from './keyof-property-keys';
import { type TUnionEvaluated } from '../union/index';
import { type TKeyOfFromMappedResult } from './keyof-from-mapped-result';
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
