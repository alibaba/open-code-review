import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { type TMappedResult, type TMappedKey } from '../mapped/index.mjs';
import { type TPick } from './pick.mjs';
type TFromPropertyKey<Type extends TSchema, Key extends PropertyKey> = {
    [_ in Key]: TPick<Type, [Key]>;
};
type TFromPropertyKeys<Type extends TSchema, PropertyKeys extends PropertyKey[], Result extends TProperties = {}> = (PropertyKeys extends [infer LeftKey extends PropertyKey, ...infer RightKeys extends PropertyKey[]] ? TFromPropertyKeys<Type, RightKeys, Result & TFromPropertyKey<Type, LeftKey>> : Result);
type TFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey> = (TFromPropertyKeys<Type, MappedKey['keys']>);
export type TPickFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey, Properties extends TProperties = TFromMappedKey<Type, MappedKey>> = (TMappedResult<Properties>);
export declare function PickFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey, Properties extends TProperties = TFromMappedKey<Type, MappedKey>>(type: Type, mappedKey: MappedKey, options?: SchemaOptions): TMappedResult<Properties>;
export {};
