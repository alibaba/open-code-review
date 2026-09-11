import type { SchemaOptions } from '../schema/index.mjs';
import type { Ensure, Evaluate } from '../helpers/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { type TMappedResult } from '../mapped/index.mjs';
import { type TKeyOfFromType } from './keyof.mjs';
type TFromProperties<Properties extends TProperties> = ({
    [K2 in keyof Properties]: TKeyOfFromType<Properties[K2]>;
});
type TFromMappedResult<MappedResult extends TMappedResult> = (Evaluate<TFromProperties<MappedResult['properties']>>);
export type TKeyOfFromMappedResult<MappedResult extends TMappedResult, Properties extends TProperties = TFromMappedResult<MappedResult>> = (Ensure<TMappedResult<Properties>>);
export declare function KeyOfFromMappedResult<MappedResult extends TMappedResult, Properties extends TProperties = TFromMappedResult<MappedResult>>(mappedResult: MappedResult, options?: SchemaOptions): TMappedResult<Properties>;
export {};
