import type { TSchema } from '../schema/index.mjs';
import type { Ensure } from '../helpers/index.mjs';
import { OptionalKind } from '../symbols/index.mjs';
import type { TMappedResult } from '../mapped/index.mjs';
import { type TOptionalFromMappedResult } from './optional-from-mapped-result.mjs';
type TRemoveOptional<T extends TSchema> = T extends TOptional<infer S> ? S : T;
type TAddOptional<T extends TSchema> = T extends TOptional<infer S> ? TOptional<S> : Ensure<TOptional<T>>;
export type TOptionalWithFlag<T extends TSchema, F extends boolean> = F extends false ? TRemoveOptional<T> : TAddOptional<T>;
export type TOptional<T extends TSchema> = T & {
    [OptionalKind]: 'Optional';
};
/** `[Json]` Creates a Optional property */
export declare function Optional<T extends TMappedResult, F extends boolean>(schema: T, enable: F): TOptionalFromMappedResult<T, F>;
/** `[Json]` Creates a Optional property */
export declare function Optional<T extends TSchema, F extends boolean>(schema: T, enable: F): TOptionalWithFlag<T, F>;
/** `[Json]` Creates a Optional property */
export declare function Optional<T extends TMappedResult>(schema: T): TOptionalFromMappedResult<T, true>;
/** `[Json]` Creates a Optional property */
export declare function Optional<T extends TSchema>(schema: T): TOptionalWithFlag<T, true>;
export {};
