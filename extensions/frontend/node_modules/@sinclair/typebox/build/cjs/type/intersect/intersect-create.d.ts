import type { TSchema } from '../schema/index';
import type { TIntersect, IntersectOptions } from './intersect-type';
export declare function IntersectCreate<T extends TSchema[]>(T: [...T], options?: IntersectOptions): TIntersect<T>;
