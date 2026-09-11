import type { TSchema } from '../schema/index';
import { type TNever } from '../never/index';
import { TIntersect, IntersectOptions } from './intersect-type';
export type Intersect<Types extends TSchema[]> = (Types extends [TSchema] ? Types[0] : Types extends [] ? TNever : TIntersect<Types>);
/** `[Json]` Creates an evaluated Intersect type */
export declare function Intersect<Types extends TSchema[]>(types: [...Types], options?: IntersectOptions): Intersect<Types>;
