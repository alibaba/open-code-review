import { Ensure } from '../helpers/index.mjs';
import type { SchemaOptions, TSchema } from '../schema/index.mjs';
import type { Static } from '../static/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface ArrayOptions extends SchemaOptions {
    /** The minimum number of items in this array */
    minItems?: number;
    /** The maximum number of items in this array */
    maxItems?: number;
    /** Should this schema contain unique items */
    uniqueItems?: boolean;
    /** A schema for which some elements should match */
    contains?: TSchema;
    /** A minimum number of contains schema matches */
    minContains?: number;
    /** A maximum number of contains schema matches */
    maxContains?: number;
}
type ArrayStatic<T extends TSchema, P extends unknown[]> = Ensure<Static<T, P>[]>;
export interface TArray<T extends TSchema = TSchema> extends TSchema, ArrayOptions {
    [Kind]: 'Array';
    static: ArrayStatic<T, this['params']>;
    type: 'array';
    items: T;
}
/** `[Json]` Creates an Array type */
export declare function Array<Type extends TSchema>(items: Type, options?: ArrayOptions): TArray<Type>;
export {};
