import type { TSchema } from '../schema/index.mjs';
import { type TNever } from '../never/index.mjs';
import { type TOptional } from '../optional/index.mjs';
import type { TReadonly } from '../readonly/index.mjs';
import { TIntersect, IntersectOptions } from './intersect-type.mjs';
type TIsIntersectOptional<Types extends TSchema[]> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? Left extends TOptional<TSchema> ? TIsIntersectOptional<Right> : false : true);
type TRemoveOptionalFromType<Type extends TSchema> = (Type extends TReadonly<infer Type extends TSchema> ? TReadonly<TRemoveOptionalFromType<Type>> : Type extends TOptional<infer Type extends TSchema> ? TRemoveOptionalFromType<Type> : Type);
type TRemoveOptionalFromRest<Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? Left extends TOptional<infer Type extends TSchema> ? TRemoveOptionalFromRest<Right, [...Result, TRemoveOptionalFromType<Type>]> : TRemoveOptionalFromRest<Right, [...Result, Left]> : Result);
type TResolveIntersect<Types extends TSchema[]> = (TIsIntersectOptional<Types> extends true ? TOptional<TIntersect<TRemoveOptionalFromRest<Types>>> : TIntersect<TRemoveOptionalFromRest<Types>>);
export type TIntersectEvaluated<Types extends TSchema[]> = (Types extends [TSchema] ? Types[0] : Types extends [] ? TNever : TResolveIntersect<Types>);
/** `[Json]` Creates an evaluated Intersect type */
export declare function IntersectEvaluated<Types extends TSchema[], Result extends TSchema = TIntersectEvaluated<Types>>(types: [...Types], options?: IntersectOptions): Result;
export {};
