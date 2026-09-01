import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export interface TUndefined extends TSchema {
    [Kind]: 'Undefined';
    static: undefined;
    type: 'undefined';
}
/** `[JavaScript]` Creates a Undefined type */
export declare function Undefined(options?: SchemaOptions): TUndefined;
