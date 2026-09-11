import type { Evaluate } from '../helpers/index.mjs';
import type { TOptional } from '../optional/index.mjs';
import type { TReadonly } from '../readonly/index.mjs';
import type { TArray } from '../array/index.mjs';
import type { TAsyncIterator } from '../async-iterator/index.mjs';
import type { TConstructor } from '../constructor/index.mjs';
import type { TEnum } from '../enum/index.mjs';
import type { TFunction } from '../function/index.mjs';
import type { TIntersect } from '../intersect/index.mjs';
import type { TImport } from '../module/index.mjs';
import type { TIterator } from '../iterator/index.mjs';
import type { TNot } from '../not/index.mjs';
import type { TObject, TProperties } from '../object/index.mjs';
import type { TPromise } from '../promise/index.mjs';
import type { TRecursive } from '../recursive/index.mjs';
import type { TRecord } from '../record/index.mjs';
import type { TRef } from '../ref/index.mjs';
import type { TTuple } from '../tuple/index.mjs';
import type { TUnion } from '../union/index.mjs';
import type { TUnsafe } from '../unsafe/index.mjs';
import type { TSchema } from '../schema/index.mjs';
import type { TTransform } from '../transform/index.mjs';
import type { TNever } from '../never/index.mjs';
type TDecodeImport<ModuleProperties extends TProperties, Key extends PropertyKey> = (Key extends keyof ModuleProperties ? TDecodeType<ModuleProperties[Key]> extends infer Type extends TSchema ? Type extends TRef<infer Ref extends string> ? TDecodeImport<ModuleProperties, Ref> : Type : TNever : TNever);
type TDecodeProperties<Properties extends TProperties> = {
    [Key in keyof Properties]: TDecodeType<Properties[Key]>;
};
type TDecodeTypes<Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? TDecodeTypes<Right, [...Result, TDecodeType<Left>]> : Result);
export type TDecodeType<Type extends TSchema> = (Type extends TOptional<infer Type extends TSchema> ? TOptional<TDecodeType<Type>> : Type extends TReadonly<infer Type extends TSchema> ? TReadonly<TDecodeType<Type>> : Type extends TTransform<infer _Input extends TSchema, infer Output> ? TUnsafe<Output> : Type extends TArray<infer Type extends TSchema> ? TArray<TDecodeType<Type>> : Type extends TAsyncIterator<infer Type extends TSchema> ? TAsyncIterator<TDecodeType<Type>> : Type extends TConstructor<infer Parameters extends TSchema[], infer InstanceType extends TSchema> ? TConstructor<TDecodeTypes<Parameters>, TDecodeType<InstanceType>> : Type extends TEnum<infer Values> ? TEnum<Values> : Type extends TFunction<infer Parameters extends TSchema[], infer ReturnType extends TSchema> ? TFunction<TDecodeTypes<Parameters>, TDecodeType<ReturnType>> : Type extends TIntersect<infer Types extends TSchema[]> ? TIntersect<TDecodeTypes<Types>> : Type extends TImport<infer ModuleProperties extends TProperties, infer Key> ? TDecodeImport<ModuleProperties, Key> : Type extends TIterator<infer Type extends TSchema> ? TIterator<TDecodeType<Type>> : Type extends TNot<infer Type extends TSchema> ? TNot<TDecodeType<Type>> : Type extends TObject<infer Properties extends TProperties> ? TObject<Evaluate<TDecodeProperties<Properties>>> : Type extends TPromise<infer Type extends TSchema> ? TPromise<TDecodeType<Type>> : Type extends TRecord<infer Key extends TSchema, infer Value extends TSchema> ? TRecord<Key, TDecodeType<Value>> : Type extends TRecursive<infer Type extends TSchema> ? TRecursive<TDecodeType<Type>> : Type extends TRef<infer Ref extends string> ? TRef<Ref> : Type extends TTuple<infer Types extends TSchema[]> ? TTuple<TDecodeTypes<Types>> : Type extends TUnion<infer Types extends TSchema[]> ? TUnion<TDecodeTypes<Types>> : Type);
export type StaticDecodeIsAny<Type> = boolean extends (Type extends TSchema ? true : false) ? true : false;
/** Creates an decoded static type from a TypeBox type */
export type StaticDecode<Type extends TSchema, Params extends unknown[] = [], Result = StaticDecodeIsAny<Type> extends true ? unknown : Static<TDecodeType<Type>, Params>> = Result;
/** Creates an encoded static type from a TypeBox type */
export type StaticEncode<Type extends TSchema, Params extends unknown[] = [], Result = Static<Type, Params>> = Result;
/** Creates a static type from a TypeBox type */
export type Static<Type extends TSchema, Params extends unknown[] = [], Result = (Type & {
    params: Params;
})['static']> = Result;
export {};
