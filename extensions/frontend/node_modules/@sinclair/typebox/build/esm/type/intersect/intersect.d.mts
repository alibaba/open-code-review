import type { TSchema } from '../schema/index.mjs';
import { type TNever } from '../never/index.mjs';
import { TIntersect, IntersectOptions } from './intersect-type.mjs';
export type Intersect<Types extends TSchema[]> = (Types extends [TSchema] ? Types[0] : Types extends [] ? TNever : TIntersect<Types>);
/** `[Json]` Creates an evaluated Intersect type */
export declare function Intersect<Types extends TSchema[]>(types: [...Types], options?: IntersectOptions): Intersect<Types>;
