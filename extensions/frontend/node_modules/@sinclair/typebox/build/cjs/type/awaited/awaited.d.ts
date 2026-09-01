import { Ensure } from '../helpers/index';
import type { TSchema, SchemaOptions } from '../schema/index';
import { type TComputed } from '../computed/index';
import { type TIntersect } from '../intersect/index';
import { type TUnion } from '../union/index';
import { type TPromise } from '../promise/index';
import { type TRef } from '../ref/index';
type TFromComputed<Target extends string, Parameters extends TSchema[]> = Ensure<(TComputed<'Awaited', [TComputed<Target, Parameters>]>)>;
type TFromRef<Ref extends string> = Ensure<TComputed<'Awaited', [TRef<Ref>]>>;
type TFromRest<Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? TFromRest<Right, [...Result, TAwaited<Left>]> : Result);
export type TAwaited<Type extends TSchema> = (Type extends TComputed<infer Target extends string, infer Parameters extends TSchema[]> ? TFromComputed<Target, Parameters> : Type extends TRef<infer Ref extends string> ? TFromRef<Ref> : Type extends TIntersect<infer Types extends TSchema[]> ? TIntersect<TFromRest<Types>> : Type extends TUnion<infer Types extends TSchema[]> ? TUnion<TFromRest<Types>> : Type extends TPromise<infer Type extends TSchema> ? TAwaited<Type> : Type);
/** `[JavaScript]` Constructs a type by recursively unwrapping Promise types */
export declare function Awaited<T extends TSchema>(type: T, options?: SchemaOptions): TAwaited<T>;
export {};
