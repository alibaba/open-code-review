import type { SchemaOptions } from '../schema/index';
import type { Ensure, Evaluate } from '../helpers/index';
import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TKeyOfFromType } from './keyof';
type TFromProperties<Properties extends TProperties> = ({
    [K2 in keyof Properties]: TKeyOfFromType<Properties[K2]>;
});
type TFromMappedResult<MappedResult extends TMappedResult> = (Evaluate<TFromProperties<MappedResult['properties']>>);
export type TKeyOfFromMappedResult<MappedResult extends TMappedResult, Properties extends TProperties = TFromMappedResult<MappedResult>> = (Ensure<TMappedResult<Properties>>);
export declare function KeyOfFromMappedResult<MappedResult extends TMappedResult, Properties extends TProperties = TFromMappedResult<MappedResult>>(mappedResult: MappedResult, options?: SchemaOptions): TMappedResult<Properties>;
export {};
