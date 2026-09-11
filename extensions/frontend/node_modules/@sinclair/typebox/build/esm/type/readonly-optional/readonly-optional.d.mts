import type { TSchema } from '../schema/index.mjs';
import { type TReadonly } from '../readonly/index.mjs';
import { type TOptional } from '../optional/index.mjs';
export type TReadonlyOptional<T extends TSchema> = TOptional<T> & TReadonly<T>;
/** `[Json]` Creates a Readonly and Optional property */
export declare function ReadonlyOptional<T extends TSchema>(schema: T): TReadonly<TOptional<T>>;
