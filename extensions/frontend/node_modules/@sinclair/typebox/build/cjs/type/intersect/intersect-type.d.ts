import type { TSchema, SchemaOptions } from '../schema/index';
import type { Static } from '../static/index';
import { Kind } from '../symbols/index';
type TIntersectStatic<T extends TSchema[], P extends unknown[], Acc extends unknown = unknown> = T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TIntersectStatic<R, P, Acc & Static<L, P>> : Acc;
export type TUnevaluatedProperties = undefined | TSchema | boolean;
export interface IntersectOptions extends SchemaOptions {
    unevaluatedProperties?: TUnevaluatedProperties;
}
export interface TIntersect<T extends TSchema[] = TSchema[]> extends TSchema, IntersectOptions {
    [Kind]: 'Intersect';
    static: TIntersectStatic<T, this['params']>;
    type?: 'object';
    allOf: [...T];
}
export {};
