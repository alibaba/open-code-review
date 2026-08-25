import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { Static } from '../static/index.mjs';
import { Kind } from '../symbols/index.mjs';
type TupleStatic<T extends TSchema[], P extends unknown[], Acc extends unknown[] = []> = T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TupleStatic<R, P, [...Acc, Static<L, P>]> : Acc;
export interface TTuple<T extends TSchema[] = TSchema[]> extends TSchema {
    [Kind]: 'Tuple';
    static: TupleStatic<T, this['params']>;
    type: 'array';
    items: T;
    additionalItems?: false;
    minItems: T['length'];
    maxItems: T['length'];
}
/** `[Json]` Creates a Tuple type */
export declare function Tuple<Types extends TSchema[]>(types: [...Types], options?: SchemaOptions): TTuple<Types>;
export {};
