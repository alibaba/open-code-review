import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/symbols.mjs';
export interface TComputed<Target extends string = string, Parameters extends TSchema[] = []> extends TSchema {
    [Kind]: 'Computed';
    target: Target;
    parameters: Parameters;
}
/** `[Internal]` Creates a deferred computed type. This type is used exclusively in modules to defer resolution of computable types that contain interior references  */
export declare function Computed<Target extends string, Parameters extends TSchema[]>(target: Target, parameters: [...Parameters], options?: SchemaOptions): TComputed<Target, Parameters>;
