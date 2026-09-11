import type { TSchema } from '../schema/index';
import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TExclude } from './exclude';
type TFromProperties<K extends TProperties, T extends TSchema> = ({
    [K2 in keyof K]: TExclude<K[K2], T>;
});
type TFromMappedResult<R extends TMappedResult, T extends TSchema> = (TFromProperties<R['properties'], T>);
export type TExcludeFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>> = (TMappedResult<P>);
export declare function ExcludeFromMappedResult<R extends TMappedResult, T extends TSchema, P extends TProperties = TFromMappedResult<R, T>>(R: R, T: T): TMappedResult<P>;
export {};
