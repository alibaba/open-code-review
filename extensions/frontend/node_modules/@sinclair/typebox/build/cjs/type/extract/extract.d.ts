import type { TSchema, SchemaOptions } from '../schema/index';
import type { AssertRest, AssertType, UnionToTuple } from '../helpers/index';
import type { TMappedResult } from '../mapped/index';
import { type TUnion } from '../union/index';
import { type Static } from '../static/index';
import { type TNever } from '../never/index';
import { type TUnionEvaluated } from '../union/index';
import { type TTemplateLiteral } from '../template-literal/index';
import { type TExtractFromMappedResult } from './extract-from-mapped-result';
import { type TExtractFromTemplateLiteral } from './extract-from-template-literal';
type TExtractRest<L extends TSchema[], R extends TSchema> = AssertRest<UnionToTuple<{
    [K in keyof L]: Static<AssertType<L[K]>> extends Static<R> ? L[K] : never;
}[number]>> extends infer R extends TSchema[] ? TUnionEvaluated<R> : never;
export type TExtract<L extends TSchema, U extends TSchema> = (L extends TUnion<infer S> ? TExtractRest<S, U> : L extends U ? L : TNever);
/** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
export declare function Extract<L extends TMappedResult, R extends TSchema>(type: L, union: R, options?: SchemaOptions): TExtractFromMappedResult<L, R>;
/** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
export declare function Extract<L extends TTemplateLiteral, R extends TSchema>(type: L, union: R, options?: SchemaOptions): TExtractFromTemplateLiteral<L, R>;
/** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
export declare function Extract<L extends TSchema, R extends TSchema>(type: L, union: R, options?: SchemaOptions): TExtract<L, R>;
export {};
