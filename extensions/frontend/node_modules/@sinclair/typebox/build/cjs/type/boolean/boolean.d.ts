import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export interface TBoolean extends TSchema {
    [Kind]: 'Boolean';
    static: boolean;
    type: 'boolean';
}
/** `[Json]` Creates a Boolean type */
export declare function Boolean(options?: SchemaOptions): TBoolean;
