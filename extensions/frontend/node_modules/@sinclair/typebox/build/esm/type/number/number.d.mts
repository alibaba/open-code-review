import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface NumberOptions extends SchemaOptions {
    exclusiveMaximum?: number;
    exclusiveMinimum?: number;
    maximum?: number;
    minimum?: number;
    multipleOf?: number;
}
export interface TNumber extends TSchema, NumberOptions {
    [Kind]: 'Number';
    static: number;
    type: 'number';
}
/** `[Json]` Creates a Number type */
export declare function Number(options?: NumberOptions): TNumber;
