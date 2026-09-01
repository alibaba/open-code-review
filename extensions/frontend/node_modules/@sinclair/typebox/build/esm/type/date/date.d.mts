import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface DateOptions extends SchemaOptions {
    /** The exclusive maximum timestamp value */
    exclusiveMaximumTimestamp?: number;
    /** The exclusive minimum timestamp value */
    exclusiveMinimumTimestamp?: number;
    /** The maximum timestamp value */
    maximumTimestamp?: number;
    /** The minimum timestamp value */
    minimumTimestamp?: number;
    /** The multiple of timestamp value */
    multipleOfTimestamp?: number;
}
export interface TDate extends TSchema, DateOptions {
    [Kind]: 'Date';
    static: Date;
    type: 'date';
}
/** `[JavaScript]` Creates a Date type */
export declare function Date(options?: DateOptions): TDate;
