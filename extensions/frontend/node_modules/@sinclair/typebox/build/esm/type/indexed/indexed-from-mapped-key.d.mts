import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { Ensure, Evaluate } from '../helpers/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { type TIndex } from './indexed.mjs';
import { type TMappedResult, type TMappedKey } from '../mapped/index.mjs';
type TMappedIndexPropertyKey<Type extends TSchema, Key extends PropertyKey> = {
    [_ in Key]: TIndex<Type, [Key]>;
};
type TMappedIndexPropertyKeys<Type extends TSchema, PropertyKeys extends PropertyKey[], Result extends TProperties = {}> = (PropertyKeys extends [infer Left extends PropertyKey, ...infer Right extends PropertyKey[]] ? TMappedIndexPropertyKeys<Type, Right, Result & TMappedIndexPropertyKey<Type, Left>> : Result);
type TMappedIndexProperties<Type extends TSchema, MappedKey extends TMappedKey> = Evaluate<TMappedIndexPropertyKeys<Type, MappedKey['keys']>>;
export type TIndexFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey, Properties extends TProperties = TMappedIndexProperties<Type, MappedKey>> = (Ensure<TMappedResult<Properties>>);
export declare function IndexFromMappedKey<Type extends TSchema, MappedKey extends TMappedKey, Properties extends TProperties = TMappedIndexProperties<Type, MappedKey>>(type: Type, mappedKey: MappedKey, options?: SchemaOptions): TMappedResult<Properties>;
export {};
