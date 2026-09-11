import type { TProperties } from '../object/index.mjs';
import { type TMappedResult } from '../mapped/index.mjs';
import { type TReadonlyWithFlag } from './readonly.mjs';
type TFromProperties<P extends TProperties, F extends boolean> = ({
    [K2 in keyof P]: TReadonlyWithFlag<P[K2], F>;
});
type TFromMappedResult<R extends TMappedResult, F extends boolean> = (TFromProperties<R['properties'], F>);
export type TReadonlyFromMappedResult<R extends TMappedResult, F extends boolean, P extends TProperties = TFromMappedResult<R, F>> = (TMappedResult<P>);
export declare function ReadonlyFromMappedResult<R extends TMappedResult, F extends boolean, P extends TProperties = TFromMappedResult<R, F>>(R: R, F: F): TMappedResult<P>;
export {};
