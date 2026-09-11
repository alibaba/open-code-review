import type { TSchema, SchemaOptions } from '../schema/index';
import type { Static } from '../static/index';
import { type TUnion } from '../union/index';
import { TMappedKey, TMappedResult } from '../mapped/index';
import { UnionToTuple } from '../helpers/index';
import { type TExtendsFromMappedKey } from './extends-from-mapped-key';
import { type TExtendsFromMappedResult } from './extends-from-mapped-result';
type TExtendsResolve<L extends TSchema, R extends TSchema, T extends TSchema, U extends TSchema> = ((Static<L> extends Static<R> ? T : U) extends infer O extends TSchema ? UnionToTuple<O> extends [infer X extends TSchema, infer Y extends TSchema] ? TUnion<[X, Y]> : O : never);
export type TExtends<L extends TSchema, R extends TSchema, T extends TSchema, F extends TSchema> = TExtendsResolve<L, R, T, F>;
/** `[Json]` Creates a Conditional type */
export declare function Extends<L extends TMappedResult, R extends TSchema, T extends TSchema, F extends TSchema>(L: L, R: R, T: T, F: F, options?: SchemaOptions): TExtendsFromMappedResult<L, R, T, F>;
/** `[Json]` Creates a Conditional type */
export declare function Extends<L extends TMappedKey, R extends TSchema, T extends TSchema, F extends TSchema>(L: L, R: R, T: T, F: F, options?: SchemaOptions): TExtendsFromMappedKey<L, R, T, F>;
/** `[Json]` Creates a Conditional type */
export declare function Extends<L extends TSchema, R extends TSchema, T extends TSchema, F extends TSchema>(L: L, R: R, T: T, F: F, options?: SchemaOptions): TExtends<L, R, T, F>;
export {};
