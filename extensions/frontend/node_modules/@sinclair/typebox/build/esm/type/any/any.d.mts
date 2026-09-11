import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TAny extends TSchema {
    [Kind]: 'Any';
    static: any;
}
/** `[Json]` Creates an Any type */
export declare function Any(options?: SchemaOptions): TAny;
