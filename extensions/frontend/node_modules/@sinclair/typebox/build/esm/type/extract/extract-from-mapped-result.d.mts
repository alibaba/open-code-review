import type { TSchema } from '../schema/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { type TMappedResult } from '../mapped/index.mjs';
import { type TExtract } from './extract.mjs';
type TFromProperties<P extends TProperties, T extends TSchema> = ({
    [K2 in keyof P]: TExtract<P[K2], T>;
});
type TFromMappedResult<R extends TMappedResult, T extends TSchema> = (TFromProperties<R['properties'], T>);
export type TExtractFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>> = (TMappedResult<P>);
export declare function ExtractFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>>(R: R, T: T): TMappedResult<P>;
export {};
