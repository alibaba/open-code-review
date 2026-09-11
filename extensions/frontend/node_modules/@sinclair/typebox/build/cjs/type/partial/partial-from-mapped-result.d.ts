import type { SchemaOptions } from '../schema/index';
import type { Ensure, Evaluate } from '../helpers/index';
import type { TProperties } from '../object/index';
import { type TMappedResult } from '../mapped/index';
import { type TPartial } from './partial';
type TFromProperties<P extends TProperties> = ({
    [K2 in keyof P]: TPartial<P[K2]>;
});
type TFromMappedResult<R extends TMappedResult> = (Evaluate<TFromProperties<R['properties']>>);
export type TPartialFromMappedResult<R extends TMappedResult, P extends TProperties = TFromMappedResult<R>> = (Ensure<TMappedResult<P>>);
export declare function PartialFromMappedResult<R extends TMappedResult, P extends TProperties = TFromMappedResult<R>>(R: R, options?: SchemaOptions): TMappedResult<P>;
export {};
