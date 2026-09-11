import type { TSchema } from '../schema/index.mjs';
import type { Static } from '../static/index.mjs';
import { Kind } from '../symbols/index.mjs';
type UnionStatic<T extends TSchema[], P extends unknown[]> = {
    [K in keyof T]: T[K] extends TSchema ? Static<T[K], P> : never;
}[number];
export interface TUnion<T extends TSchema[] = TSchema[]> extends TSchema {
    [Kind]: 'Union';
    static: UnionStatic<T, this['params']>;
    anyOf: T;
}
export {};
