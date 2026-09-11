import type { TSchema, SchemaOptions } from '../schema/index';
import { type TIntrinsic } from './intrinsic';
export type TCapitalize<T extends TSchema> = TIntrinsic<T, 'Capitalize'>;
/** `[Json]` Intrinsic function to Capitalize LiteralString types */
export declare function Capitalize<T extends TSchema>(T: T, options?: SchemaOptions): TCapitalize<T>;
