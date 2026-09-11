import type { TSchema } from '../schema/index';
import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TExtract } from './extract';
type TFromProperties<P extends TProperties, T extends TSchema> = ({
    [K2 in keyof P]: TExtract<P[K2], T>;
});
type TFromMappedResult<R extends TMappedResult, T extends TSchema> = (TFromProperties<R['properties'], T>);
export type TExtractFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>> = (TMappedResult<P>);
export declare function ExtractFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>>(R: R, T: T): TMappedResult<P>;
export {};
