import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import { type TNever } from '../never/index.mjs';
import type { TUnion } from './union-type.mjs';
export type Union<T extends TSchema[]> = (T extends [] ? TNever : T extends [TSchema] ? T[0] : TUnion<T>);
/** `[Json]` Creates a Union type */
export declare function Union<Types extends TSchema[]>(types: [...Types], options?: SchemaOptions): Union<Types>;
