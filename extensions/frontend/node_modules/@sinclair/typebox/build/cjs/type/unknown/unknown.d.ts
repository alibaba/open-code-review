import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export interface TUnknown extends TSchema {
    [Kind]: 'Unknown';
    static: unknown;
}
/** `[Json]` Creates an Unknown type */
export declare function Unknown(options?: SchemaOptions): TUnknown;
