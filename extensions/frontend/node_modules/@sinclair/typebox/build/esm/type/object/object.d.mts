import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { Static } from '../static/index.mjs';
import type { Evaluate, UnionToTuple } from '../helpers/index.mjs';
import type { TReadonly } from '../readonly/index.mjs';
import type { TOptional } from '../optional/index.mjs';
import { Kind } from '../symbols/index.mjs';
type ReadonlyOptionalPropertyKeys<T extends TProperties> = {
    [K in keyof T]: T[K] extends TReadonly<TSchema> ? (T[K] extends TOptional<T[K]> ? K : never) : never;
}[keyof T];
type ReadonlyPropertyKeys<T extends TProperties> = {
    [K in keyof T]: T[K] extends TReadonly<TSchema> ? (T[K] extends TOptional<T[K]> ? never : K) : never;
}[keyof T];
type OptionalPropertyKeys<T extends TProperties> = {
    [K in keyof T]: T[K] extends TOptional<TSchema> ? (T[K] extends TReadonly<T[K]> ? never : K) : never;
}[keyof T];
type RequiredPropertyKeys<T extends TProperties> = keyof Omit<T, ReadonlyOptionalPropertyKeys<T> | ReadonlyPropertyKeys<T> | OptionalPropertyKeys<T>>;
type ObjectStaticProperties<T extends TProperties, R extends Record<keyof any, unknown>> = Evaluate<(Readonly<Partial<Pick<R, ReadonlyOptionalPropertyKeys<T>>>> & Readonly<Pick<R, ReadonlyPropertyKeys<T>>> & Partial<Pick<R, OptionalPropertyKeys<T>>> & Required<Pick<R, RequiredPropertyKeys<T>>>)>;
type ObjectStatic<T extends TProperties, P extends unknown[]> = ObjectStaticProperties<T, {
    [K in keyof T]: Static<T[K], P>;
}>;
export type TPropertyKey = string | number;
export type TProperties = Record<TPropertyKey, TSchema>;
type TIsLiteralString<Type extends string> = ([
    Type
] extends [string] ? [string] extends [Type] ? false : true : false);
type IsRequiredArrayLiteralConstant<RequiredTuple extends string[]> = (RequiredTuple extends [infer Left extends string, ...infer _ extends string[]] ? TIsLiteralString<Left> : false);
type TRequiredArray<Properties extends TProperties, RequiredProperties extends TProperties = {
    [Key in keyof Properties as Properties[Key] extends TOptional<Properties[Key]> ? never : Key]: Properties[Key];
}, RequiredUnion extends string = Extract<keyof RequiredProperties, string>, RequiredTuple extends string[] = UnionToTuple<RequiredUnion>, Result extends string[] | undefined = (IsRequiredArrayLiteralConstant<RequiredTuple> extends true ? RequiredTuple : string[] | undefined)> = Result;
export type TAdditionalProperties = undefined | TSchema | boolean;
export interface ObjectOptions extends SchemaOptions {
    /** Additional property constraints for this object */
    additionalProperties?: TAdditionalProperties;
    /** The minimum number of properties allowed on this object */
    minProperties?: number;
    /** The maximum number of properties allowed on this object */
    maxProperties?: number;
}
export interface TObject<T extends TProperties = TProperties> extends TSchema, ObjectOptions {
    [Kind]: 'Object';
    static: ObjectStatic<T, this['params']>;
    additionalProperties?: TAdditionalProperties;
    type: 'object';
    properties: T;
    required: TRequiredArray<T>;
}
/** `[Json]` Creates an Object type */
declare function _Object_<T extends TProperties>(properties: T, options?: ObjectOptions): TObject<T>;
/** `[Json]` Creates an Object type */
export declare var Object: typeof _Object_;
export {};
