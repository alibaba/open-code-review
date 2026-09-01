import type { SchemaOptions } from '../schema/index';
import type { TProperties } from '../object/index';
import { Assert } from '../helpers/index';
import { type TMappedResult, type TMappedKey } from '../mapped/index';
import { type TIntrinsic, type IntrinsicMode } from './intrinsic';
import { type TLiteral, type TLiteralValue } from '../literal/index';
type TMappedIntrinsicPropertyKey<K extends PropertyKey, M extends IntrinsicMode> = {
    [_ in K]: TIntrinsic<TLiteral<Assert<K, TLiteralValue>>, M>;
};
type TMappedIntrinsicPropertyKeys<K extends PropertyKey[], M extends IntrinsicMode, Acc extends TProperties = {}> = (K extends [infer L extends PropertyKey, ...infer R extends PropertyKey[]] ? TMappedIntrinsicPropertyKeys<R, M, Acc & TMappedIntrinsicPropertyKey<L, M>> : Acc);
type TMappedIntrinsicProperties<K extends TMappedKey, M extends IntrinsicMode> = (TMappedIntrinsicPropertyKeys<K['keys'], M>);
export type TIntrinsicFromMappedKey<K extends TMappedKey, M extends IntrinsicMode, P extends TProperties = TMappedIntrinsicProperties<K, M>> = (TMappedResult<P>);
export declare function IntrinsicFromMappedKey<K extends TMappedKey, M extends IntrinsicMode, P extends TProperties = TMappedIntrinsicProperties<K, M>>(T: K, M: M, options: SchemaOptions): TMappedResult<P>;
export {};
