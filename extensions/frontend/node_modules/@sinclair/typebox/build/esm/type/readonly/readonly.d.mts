import type { TSchema } from '../schema/index.mjs';
import type { Ensure } from '../helpers/index.mjs';
import { ReadonlyKind } from '../symbols/index.mjs';
import type { TMappedResult } from '../mapped/index.mjs';
import { type TReadonlyFromMappedResult } from './readonly-from-mapped-result.mjs';
type TRemoveReadonly<T extends TSchema> = T extends TReadonly<infer S> ? S : T;
type TAddReadonly<T extends TSchema> = T extends TReadonly<infer S> ? TReadonly<S> : Ensure<TReadonly<T>>;
export type TReadonlyWithFlag<T extends TSchema, F extends boolean> = F extends false ? TRemoveReadonly<T> : TAddReadonly<T>;
export type TReadonly<T extends TSchema> = T & {
    [ReadonlyKind]: 'Readonly';
};
/** `[Json]` Creates a Readonly property */
export declare function Readonly<T extends TMappedResult, F extends boolean>(schema: T, enable: F): TReadonlyFromMappedResult<T, F>;
/** `[Json]` Creates a Readonly property */
export declare function Readonly<T extends TSchema, F extends boolean>(schema: T, enable: F): TReadonlyWithFlag<T, F>;
/** `[Json]` Creates a Readonly property */
export declare function Readonly<T extends TMappedResult>(schema: T): TReadonlyFromMappedResult<T, true>;
/** `[Json]` Creates a Readonly property */
export declare function Readonly<T extends TSchema>(schema: T): TReadonlyWithFlag<T, true>;
export {};
