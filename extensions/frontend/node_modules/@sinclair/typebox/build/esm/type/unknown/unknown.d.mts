import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TUnknown extends TSchema {
    [Kind]: 'Unknown';
    static: unknown;
}
/** `[Json]` Creates an Unknown type */
export declare function Unknown(options?: SchemaOptions): TUnknown;
