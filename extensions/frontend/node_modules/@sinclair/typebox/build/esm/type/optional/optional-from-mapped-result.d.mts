import type { TProperties } from '../object/index.mjs';
import { type TMappedResult } from '../mapped/index.mjs';
import { type TOptionalWithFlag } from './optional.mjs';
type TFromProperties<P extends TProperties, F extends boolean> = ({
    [K2 in keyof P]: TOptionalWithFlag<P[K2], F>;
});
type TFromMappedResult<R extends TMappedResult, F extends boolean> = (TFromProperties<R['properties'], F>);
export type TOptionalFromMappedResult<R extends TMappedResult, F extends boolean, P extends TProperties = TFromMappedResult<R, F>> = (TMappedResult<P>);
export declare function OptionalFromMappedResult<R extends TMappedResult, F extends boolean, P extends TProperties = TFromMappedResult<R, F>>(R: R, F: F): TMappedResult<P>;
export {};
