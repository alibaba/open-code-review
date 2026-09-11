import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { type TMappedResult } from '../mapped/index.mjs';
import { type TIndexPropertyKeys } from './indexed-property-keys.mjs';
import { type TIndex } from './index.mjs';
type TFromProperties<Type extends TSchema, Properties extends TProperties> = ({
    [K2 in keyof Properties]: TIndex<Type, TIndexPropertyKeys<Properties[K2]>>;
});
type TFromMappedResult<Type extends TSchema, MappedResult extends TMappedResult> = (TFromProperties<Type, MappedResult['properties']>);
export type TIndexFromMappedResult<Type extends TSchema, MappedResult extends TMappedResult, Properties extends TProperties = TFromMappedResult<Type, MappedResult>> = (TMappedResult<Properties>);
export declare function IndexFromMappedResult<Type extends TSchema, MappedResult extends TMappedResult, Properties extends TProperties = TFromMappedResult<Type, MappedResult>>(type: Type, mappedResult: MappedResult, options?: SchemaOptions): TMappedResult<Properties>;
export {};
