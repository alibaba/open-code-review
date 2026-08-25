import type { TSchema, SchemaOptions } from '../schema/index';
import type { Static } from '../static/index';
import { Kind } from '../symbols/index';
export interface TPromise<T extends TSchema = TSchema> extends TSchema {
    [Kind]: 'Promise';
    static: Promise<Static<T, this['params']>>;
    type: 'Promise';
    item: TSchema;
}
/** `[JavaScript]` Creates a Promise type */
export declare function Promise<T extends TSchema>(item: T, options?: SchemaOptions): TPromise<T>;
