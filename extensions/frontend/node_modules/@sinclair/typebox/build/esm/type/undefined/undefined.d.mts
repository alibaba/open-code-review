import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TUndefined extends TSchema {
    [Kind]: 'Undefined';
    static: undefined;
    type: 'undefined';
}
/** `[JavaScript]` Creates a Undefined type */
export declare function Undefined(options?: SchemaOptions): TUndefined;
