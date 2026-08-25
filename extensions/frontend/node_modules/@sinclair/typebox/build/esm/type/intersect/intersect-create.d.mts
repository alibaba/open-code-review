import type { TSchema } from '../schema/index.mjs';
import type { TIntersect, IntersectOptions } from './intersect-type.mjs';
export declare function IntersectCreate<T extends TSchema[]>(T: [...T], options?: IntersectOptions): TIntersect<T>;
