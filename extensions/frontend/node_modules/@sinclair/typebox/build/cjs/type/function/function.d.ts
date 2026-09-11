import type { TSchema, SchemaOptions } from '../schema/index';
import type { Static } from '../static/index';
import type { Ensure } from '../helpers/index';
import type { TReadonlyOptional } from '../readonly-optional/index';
import type { TReadonly } from '../readonly/index';
import type { TOptional } from '../optional/index';
import { Kind } from '../symbols/index';
type StaticReturnType<U extends TSchema, P extends unknown[]> = Static<U, P>;
type StaticParameter<T extends TSchema, P extends unknown[]> = T extends TReadonlyOptional<T> ? [Readonly<Static<T, P>>?] : T extends TReadonly<T> ? [Readonly<Static<T, P>>] : T extends TOptional<T> ? [Static<T, P>?] : [
    Static<T, P>
];
type StaticParameters<T extends TSchema[], P extends unknown[], Acc extends unknown[] = []> = (T extends [infer L extends TSchema, ...infer R extends TSchema[]] ? StaticParameters<R, P, [...Acc, ...StaticParameter<L, P>]> : Acc);
type StaticFunction<T extends TSchema[], U extends TSchema, P extends unknown[]> = Ensure<(...param: StaticParameters<T, P>) => StaticReturnType<U, P>>;
export interface TFunction<T extends TSchema[] = TSchema[], U extends TSchema = TSchema> extends TSchema {
    [Kind]: 'Function';
    static: StaticFunction<T, U, this['params']>;
    type: 'Function';
    parameters: T;
    returns: U;
}
/** `[JavaScript]` Creates a Function type */
export declare function Function<T extends TSchema[], U extends TSchema>(parameters: [...T], returns: U, options?: SchemaOptions): TFunction<T, U>;
export {};
