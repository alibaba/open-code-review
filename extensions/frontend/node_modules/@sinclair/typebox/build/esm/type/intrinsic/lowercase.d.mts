import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { type TIntrinsic } from './intrinsic.mjs';
export type TLowercase<T extends TSchema> = TIntrinsic<T, 'Lowercase'>;
/** `[Json]` Intrinsic function to Lowercase LiteralString types */
export declare function Lowercase<T extends TSchema>(T: T, options?: SchemaOptions): TLowercase<T>;
