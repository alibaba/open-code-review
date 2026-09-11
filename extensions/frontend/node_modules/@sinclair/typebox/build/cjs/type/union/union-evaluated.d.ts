import type { SchemaOptions, TSchema } from '../schema/index';
import { type TNever } from '../never/index';
import { type TOptional } from '../optional/index';
import type { TReadonly } from '../readonly/index';
import type { TUnion } from './union-type';
type TIsUnionOptional<Types extends TSchema[]> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? Left extends TOptional<TSchema> ? true : TIsUnionOptional<Right> : false);
type TRemoveOptionalFromRest<Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? Left extends TOptional<infer S extends TSchema> ? TRemoveOptionalFromRest<Right, [...Result, TRemoveOptionalFromType<S>]> : TRemoveOptionalFromRest<Right, [...Result, Left]> : Result);
type TRemoveOptionalFromType<Type extends TSchema> = (Type extends TReadonly<infer Type extends TSchema> ? TReadonly<TRemoveOptionalFromType<Type>> : Type extends TOptional<infer Type extends TSchema> ? TRemoveOptionalFromType<Type> : Type);
type TResolveUnion<Types extends TSchema[], Result extends TSchema[] = TRemoveOptionalFromRest<Types>, IsOptional extends boolean = TIsUnionOptional<Types>> = (IsOptional extends true ? TOptional<TUnion<Result>> : TUnion<Result>);
export type TUnionEvaluated<Types extends TSchema[]> = (Types extends [TSchema] ? Types[0] : Types extends [] ? TNever : TResolveUnion<Types>);
/** `[Json]` Creates an evaluated Union type */
export declare function UnionEvaluated<Types extends TSchema[], Result = TUnionEvaluated<Types>>(T: [...Types], options?: SchemaOptions): Result;
export {};
