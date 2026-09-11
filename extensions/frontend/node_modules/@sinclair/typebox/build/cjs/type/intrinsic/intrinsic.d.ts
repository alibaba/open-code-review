import type { TSchema, SchemaOptions } from '../schema/index';
import { type TTemplateLiteral, type TTemplateLiteralKind } from '../template-literal/index';
import { type TIntrinsicFromMappedKey } from './intrinsic-from-mapped-key';
import { type TLiteral } from '../literal/index';
import { type TUnion } from '../union/index';
import { type TMappedKey } from '../mapped/index';
export type IntrinsicMode = 'Uppercase' | 'Lowercase' | 'Capitalize' | 'Uncapitalize';
type TFromTemplateLiteral<T extends TTemplateLiteralKind[], M extends IntrinsicMode> = M extends IntrinsicMode ? T extends [infer L extends TTemplateLiteralKind, ...infer R extends TTemplateLiteralKind[]] ? [TIntrinsic<L, M>, ...TFromTemplateLiteral<R, M>] : T : T;
type TFromLiteralValue<T, M extends IntrinsicMode> = (T extends string ? M extends 'Uncapitalize' ? Uncapitalize<T> : M extends 'Capitalize' ? Capitalize<T> : M extends 'Uppercase' ? Uppercase<T> : M extends 'Lowercase' ? Lowercase<T> : string : T);
type TFromRest<T extends TSchema[], M extends IntrinsicMode, Acc extends TSchema[] = []> = T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TFromRest<R, M, [...Acc, TIntrinsic<L, M>]> : Acc;
export type TIntrinsic<T extends TSchema, M extends IntrinsicMode> = T extends TMappedKey ? TIntrinsicFromMappedKey<T, M> : T extends TTemplateLiteral<infer S> ? TTemplateLiteral<TFromTemplateLiteral<S, M>> : T extends TUnion<infer S> ? TUnion<TFromRest<S, M>> : T extends TLiteral<infer S> ? TLiteral<TFromLiteralValue<S, M>> : T;
/** Applies an intrinsic string manipulation to the given type. */
export declare function Intrinsic<T extends TMappedKey, M extends IntrinsicMode>(schema: T, mode: M, options?: SchemaOptions): TIntrinsicFromMappedKey<T, M>;
/** Applies an intrinsic string manipulation to the given type. */
export declare function Intrinsic<T extends TSchema, M extends IntrinsicMode>(schema: T, mode: M, options?: SchemaOptions): TIntrinsic<T, M>;
export {};
