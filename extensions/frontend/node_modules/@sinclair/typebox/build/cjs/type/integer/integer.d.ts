import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export interface IntegerOptions extends SchemaOptions {
    exclusiveMaximum?: number;
    exclusiveMinimum?: number;
    maximum?: number;
    minimum?: number;
    multipleOf?: number;
}
export interface TInteger extends TSchema, IntegerOptions {
    [Kind]: 'Integer';
    static: number;
    type: 'integer';
}
/** `[Json]` Creates an Integer type */
export declare function Integer(options?: IntegerOptions): TInteger;
