import type { TSchema, SchemaOptions } from '../schema/index';
import type { TProperties } from '../object/index';
import type { Assert } from '../helpers/index';
import { type TMappedResult, type TMappedKey } from '../mapped/index';
import { type TLiteral, type TLiteralValue } from '../literal/index';
import { type TExtends } from './extends';
type TFromPropertyKey<K extends PropertyKey, U extends TSchema, L extends TSchema, R extends TSchema> = {
    [_ in K]: TExtends<TLiteral<Assert<K, TLiteralValue>>, U, L, R>;
};
type TFromPropertyKeys<K extends PropertyKey[], U extends TSchema, L extends TSchema, R extends TSchema, Acc extends TProperties = {}> = (K extends [infer LK extends PropertyKey, ...infer RK extends PropertyKey[]] ? TFromPropertyKeys<RK, U, L, R, Acc & TFromPropertyKey<LK, U, L, R>> : Acc);
type TFromMappedKey<K extends TMappedKey, U extends TSchema, L extends TSchema, R extends TSchema> = (TFromPropertyKeys<K['keys'], U, L, R>);
export type TExtendsFromMappedKey<T extends TMappedKey, U extends TSchema, L extends TSchema, R extends TSchema, P extends TProperties = TFromMappedKey<T, U, L, R>> = (TMappedResult<P>);
export declare function ExtendsFromMappedKey<T extends TMappedKey, U extends TSchema, L extends TSchema, R extends TSchema, P extends TProperties = TFromMappedKey<T, U, L, R>>(T: T, U: U, L: L, R: R, options?: SchemaOptions): TMappedResult<P>;
export {};
