import type { TSchema } from '../schema/index.mjs';
import type { TIntersect } from '../intersect/index.mjs';
import type { TUnion } from '../union/index.mjs';
import type { TTuple } from '../tuple/index.mjs';
type TRestResolve<T extends TSchema> = T extends TIntersect<infer S extends TSchema[]> ? S : T extends TUnion<infer S extends TSchema[]> ? S : T extends TTuple<infer S extends TSchema[]> ? S : [
];
export type TRest<T extends TSchema> = TRestResolve<T>;
/** `[Json]` Extracts interior Rest elements from Tuple, Intersect and Union types */
export declare function Rest<T extends TSchema>(T: T): TRest<T>;
export {};
