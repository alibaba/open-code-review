import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export interface TNull extends TSchema {
    [Kind]: 'Null';
    static: null;
    type: 'null';
}
/** `[Json]` Creates a Null type */
export declare function Null(options?: SchemaOptions): TNull;
