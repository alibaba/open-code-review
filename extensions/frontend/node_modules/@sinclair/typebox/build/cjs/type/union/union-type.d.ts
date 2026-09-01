import type { TSchema } from '../schema/index';
import type { Static } from '../static/index';
import { Kind } from '../symbols/index';
type UnionStatic<T extends TSchema[], P extends unknown[]> = {
    [K in keyof T]: T[K] extends TSchema ? Static<T[K], P> : never;
}[number];
export interface TUnion<T extends TSchema[] = TSchema[]> extends TSchema {
    [Kind]: 'Union';
    static: UnionStatic<T, this['params']>;
    anyOf: T;
}
export {};
