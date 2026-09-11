import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TNull extends TSchema {
    [Kind]: 'Null';
    static: null;
    type: 'null';
}
/** `[Json]` Creates a Null type */
export declare function Null(options?: SchemaOptions): TNull;
