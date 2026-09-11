import type { TSchema } from '../schema/index';
import { type TReadonly } from '../readonly/index';
import { type TOptional } from '../optional/index';
export type TReadonlyOptional<T extends TSchema> = TOptional<T> & TReadonly<T>;
/** `[Json]` Creates a Readonly and Optional property */
export declare function ReadonlyOptional<T extends TSchema>(schema: T): TReadonly<TOptional<T>>;
