import type { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export interface UnsafeOptions extends SchemaOptions {
    [Kind]?: string;
}
export interface TUnsafe<T> extends TSchema {
    [Kind]: string;
    static: T;
}
/** `[Json]` Creates a Unsafe type that will infers as the generic argument T */
export declare function Unsafe<T>(options?: UnsafeOptions): TUnsafe<T>;
