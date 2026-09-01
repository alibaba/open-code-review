import type { TSchema, SchemaOptions } from '../schema/index';
import type { TProperties } from '../object/index';
import { type TMappedResult, type TMappedKey } from '../mapped/index';
import { type TOmit } from './omit';
type TFromPropertyKey<Type extends TSchema, Key extends PropertyKey> = {
    [_ in Key]: TOmit<Type, [Key]>;
};
type TFromPropertyKeys<Type extends TSchema, PropertyKeys extends PropertyKey[], Result extends TProperties = {}> = (PropertyKeys extends [infer LK extends PropertyKey, ...infer RK extends PropertyKey[]] ? TFromPropertyKeys<Type, RK, Result & TFromPropertyKey<Type, LK>> : Result);
type TFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey> = (TFromPropertyKeys<Type, MappedKey['keys']>);
export type TOmitFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey, Properties extends TProperties = TFromMappedKey<Type, MappedKey>> = (TMappedResult<Properties>);
export declare function OmitFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey, Properties extends TProperties = TFromMappedKey<Type, MappedKey>>(type: Type, mappedKey: MappedKey, options?: SchemaOptions): TMappedResult<Properties>;
export {};
