import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export interface TNever extends TSchema {
    [Kind]: 'Never';
    static: never;
    not: {};
}
/** `[Json]` Creates a Never type */
export declare function Never(options?: SchemaOptions): TNever;
