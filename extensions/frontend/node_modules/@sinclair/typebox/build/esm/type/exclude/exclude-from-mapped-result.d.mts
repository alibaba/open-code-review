import type { TSchema } from '../schema/index.mjs';
import type { TProperties } from '../object/index.mjs';
import { type TMappedResult } from '../mapped/index.mjs';
import { type TExclude } from './exclude.mjs';
type TFromProperties<K extends TProperties, T extends TSchema> = ({
    [K2 in keyof K]: TExclude<K[K2], T>;
});
type TFromMappedResult<R extends TMappedResult, T extends TSchema> = (TFromProperties<R['properties'], T>);
export type TExcludeFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>> = (TMappedResult<P>);
export declare function ExcludeFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>>(R: R, T: T): TMappedResult<P>;
export {};
