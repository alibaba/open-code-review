import type { TSchema, SchemaOptions } from '../schema/index';
import type { Static } from '../static/index';
import { Kind } from '../symbols/index';
export interface TAsyncIterator<T extends TSchema = TSchema> extends TSchema {
    [Kind]: 'AsyncIterator';
    static: AsyncIterableIterator<Static<T, this['params']>>;
    type: 'AsyncIterator';
    items: T;
}
/** `[JavaScript]` Creates a AsyncIterator type */
export declare function AsyncIterator<T extends TSchema>(items: T, options?: SchemaOptions): TAsyncIterator<T>;
