import type { TSchema, SchemaOptions } from '../schema/index';
import { type TIntrinsic } from './intrinsic';
export type TUppercase<T extends TSchema> = TIntrinsic<T, 'Uppercase'>;
/** `[Json]` Intrinsic function to Uppercase LiteralString types */
export declare function Uppercase<T extends TSchema>(T: T, options?: SchemaOptions): TUppercase<T>;
