import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TOptionalWithFlag } from './optional';
type TFromProperties<P extends TProperties, F extends boolean> = ({
    [K2 in keyof P]: TOptionalWithFlag<P[K2], F>;
});
type TFromMappedResult<R extends TMappedResult, F extends boolean> = (TFromProperties<R['properties'], F>);
export type TOptionalFromMappedResult<R extends TMappedResult, F extends boolean, P extends TProperties = TFromMappedResult<R, F>> = (TMappedResult<P>);
export declare function OptionalFromMappedResult<R extends TMappedResult, F extends boolean, P extends TProperties = TFromMappedResult<R, F>>(R: R, F: F): TMappedResult<P>;
export {};
