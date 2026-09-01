import type { SchemaOptions } from '../schema/index';
import type { Ensure, Evaluate } from '../helpers/index';
import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TRequired } from './required';
type TFromProperties<P extends TProperties> = ({
    [K2 in keyof P]: TRequired<P[K2]>;
});
type TFromMappedResult<R extends TMappedResult> = (Evaluate<TFromProperties<R['properties']>>);
export type TRequiredFromMappedResult<R extends TMappedResult, P extends TProperties = TFromMappedResult<R>> = (Ensure<TMappedResult<P>>);
export declare function RequiredFromMappedResult<R extends TMappedResult, P extends TProperties = TFromMappedResult<R>>(R: R, options?: SchemaOptions): TMappedResult<P>;
export {};
