import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { Static } from '../static/index.mjs';
import { Kind } from '../symbols/index.mjs';
export interface TIterator<T extends TSchema = TSchema> extends TSchema {
    [Kind]: 'Iterator';
    static: IterableIterator<Static<T, this['params']>>;
    type: 'Iterator';
    items: T;
}
/** `[JavaScript]` Creates an Iterator type */
export declare function Iterator<T extends TSchema>(items: T, options?: SchemaOptions): TIterator<T>;
