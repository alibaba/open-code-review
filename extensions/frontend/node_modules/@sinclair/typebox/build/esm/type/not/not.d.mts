import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { Static } from '../static/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TNot<T extends TSchema = TSchema> extends TSchema {
    [Kind]: 'Not';
    static: T extends TNot<infer U> ? Static<U> : unknown;
    not: T;
}
/** `[Json]` Creates a Not type */
export declare function Not<Type extends TSchema>(type: Type, options?: SchemaOptions): TNot<Type>;
