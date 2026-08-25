import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TNever extends TSchema {
    [Kind]: 'Never';
    static: never;
    not: {};
}
/** `[Json]` Creates a Never type */
export declare function Never(options?: SchemaOptions): TNever;
