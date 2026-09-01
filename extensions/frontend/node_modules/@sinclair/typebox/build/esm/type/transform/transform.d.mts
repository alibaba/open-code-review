import type { TSchema } from '../schema/index.mjs';
import type { Static, StaticDecode } from '../static/index.mjs';
import { TransformKind } from '../symbols/index.mjs';
export declare class TransformDecodeBuilder<T extends TSchema> {
    private readonly schema;
    constructor(schema: T);
    Decode<U extends unknown, D extends TransformFunction<StaticDecode<T>, U>>(decode: D): TransformEncodeBuilder<T, D>;
}
export declare class TransformEncodeBuilder<T extends TSchema, D extends TransformFunction> {
    private readonly schema;
    private readonly decode;
    constructor(schema: T, decode: D);
    private EncodeTransform;
    private EncodeSchema;
    Encode<E extends TransformFunction<ReturnType<D>, StaticDecode<T>>>(encode: E): TTransform<T, ReturnType<D>>;
}
type TransformStatic<T extends TSchema, P extends unknown[] = []> = T extends TTransform<infer _, infer S> ? S : Static<T, P>;
export type TransformFunction<T = any, U = any> = (value: T) => U;
export interface TransformOptions<I extends TSchema = TSchema, O extends unknown = unknown> {
    Decode: TransformFunction<StaticDecode<I>, O>;
    Encode: TransformFunction<O, StaticDecode<I>>;
}
export interface TTransform<I extends TSchema = TSchema, O extends unknown = unknown> extends TSchema {
    static: TransformStatic<I, this['params']>;
    [TransformKind]: TransformOptions<I, O>;
    [key: string]: any;
}
/** `[Json]` Creates a Transform type */
export declare function Transform<I extends TSchema>(schema: I): TransformDecodeBuilder<I>;
export {};
