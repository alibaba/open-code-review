import { Ensure, Evaluate } from '../helpers/index.mjs';
import { TSchema } from '../schema/index.mjs';
import { TArray } from '../array/index.mjs';
import { TAsyncIterator } from '../async-iterator/index.mjs';
import { TConstructor } from '../constructor/index.mjs';
import { TEnum, TEnumRecord } from '../enum/index.mjs';
import { TFunction } from '../function/index.mjs';
import { TIntersect } from '../intersect/index.mjs';
import { TIterator } from '../iterator/index.mjs';
import { TObject, TProperties } from '../object/index.mjs';
import { TOptional } from '../optional/index.mjs';
import { TRecord } from '../record/index.mjs';
import { TReadonly } from '../readonly/index.mjs';
import { TRef } from '../ref/index.mjs';
import { TTuple } from '../tuple/index.mjs';
import { TUnion } from '../union/index.mjs';
import { Static } from '../static/index.mjs';
import { TRecursive } from '../recursive/index.mjs';
type TInferArray<ModuleProperties extends TProperties, Type extends TSchema> = (Ensure<Array<TInfer<ModuleProperties, Type>>>);
type TInferAsyncIterator<ModuleProperties extends TProperties, Type extends TSchema> = (Ensure<AsyncIterableIterator<TInfer<ModuleProperties, Type>>>);
type TInferConstructor<ModuleProperties extends TProperties, Parameters extends TSchema[], InstanceType extends TSchema> = Ensure<new (...args: TInferTuple<ModuleProperties, Parameters>) => TInfer<ModuleProperties, InstanceType>>;
type TInferFunction<ModuleProperties extends TProperties, Parameters extends TSchema[], ReturnType extends TSchema> = Ensure<(...args: TInferTuple<ModuleProperties, Parameters>) => TInfer<ModuleProperties, ReturnType>>;
type TInferIterator<ModuleProperties extends TProperties, Type extends TSchema> = (Ensure<IterableIterator<TInfer<ModuleProperties, Type>>>);
type TInferIntersect<ModuleProperties extends TProperties, Types extends TSchema[], Result extends unknown = unknown> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? TInferIntersect<ModuleProperties, Right, Result & TInfer<ModuleProperties, Left>> : Result);
type ReadonlyOptionalPropertyKeys<Properties extends TProperties> = {
    [Key in keyof Properties]: Properties[Key] extends TReadonly<TSchema> ? (Properties[Key] extends TOptional<Properties[Key]> ? Key : never) : never;
}[keyof Properties];
type ReadonlyPropertyKeys<Source extends TProperties> = {
    [Key in keyof Source]: Source[Key] extends TReadonly<TSchema> ? (Source[Key] extends TOptional<Source[Key]> ? never : Key) : never;
}[keyof Source];
type OptionalPropertyKeys<Source extends TProperties> = {
    [Key in keyof Source]: Source[Key] extends TOptional<TSchema> ? (Source[Key] extends TReadonly<Source[Key]> ? never : Key) : never;
}[keyof Source];
type RequiredPropertyKeys<Source extends TProperties> = keyof Omit<Source, ReadonlyOptionalPropertyKeys<Source> | ReadonlyPropertyKeys<Source> | OptionalPropertyKeys<Source>>;
type InferPropertiesWithModifiers<Properties extends TProperties, Source extends Record<keyof any, unknown>> = Evaluate<(Readonly<Partial<Pick<Source, ReadonlyOptionalPropertyKeys<Properties>>>> & Readonly<Pick<Source, ReadonlyPropertyKeys<Properties>>> & Partial<Pick<Source, OptionalPropertyKeys<Properties>>> & Required<Pick<Source, RequiredPropertyKeys<Properties>>>)>;
type InferProperties<ModuleProperties extends TProperties, Properties extends TProperties> = InferPropertiesWithModifiers<Properties, {
    [K in keyof Properties]: TInfer<ModuleProperties, Properties[K]>;
}>;
type TInferObject<ModuleProperties extends TProperties, Properties extends TProperties> = (InferProperties<ModuleProperties, Properties>);
type TInferTuple<ModuleProperties extends TProperties, Types extends TSchema[], Result extends unknown[] = []> = (Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TInferTuple<ModuleProperties, R, [...Result, TInfer<ModuleProperties, L>]> : Result);
type TInferRecord<ModuleProperties extends TProperties, Key extends TSchema, Type extends TSchema, InferredKey extends PropertyKey = TInfer<ModuleProperties, Key> extends infer Key extends PropertyKey ? Key : never, InferedType extends unknown = TInfer<ModuleProperties, Type>> = Ensure<{
    [_ in InferredKey]: InferedType;
}>;
type TInferRef<ModuleProperties extends TProperties, Ref extends string> = (Ref extends keyof ModuleProperties ? TInfer<ModuleProperties, ModuleProperties[Ref]> : unknown);
type TInferUnion<ModuleProperties extends TProperties, Types extends TSchema[], Result extends unknown = never> = (Types extends [infer L extends TSchema, ...infer R extends TSchema[]] ? TInferUnion<ModuleProperties, R, Result | TInfer<ModuleProperties, L>> : Result);
type TInfer<ModuleProperties extends TProperties, Type extends TSchema> = (Type extends TArray<infer Type extends TSchema> ? TInferArray<ModuleProperties, Type> : Type extends TAsyncIterator<infer Type extends TSchema> ? TInferAsyncIterator<ModuleProperties, Type> : Type extends TConstructor<infer Parameters extends TSchema[], infer InstanceType extends TSchema> ? TInferConstructor<ModuleProperties, Parameters, InstanceType> : Type extends TFunction<infer Parameters extends TSchema[], infer ReturnType extends TSchema> ? TInferFunction<ModuleProperties, Parameters, ReturnType> : Type extends TIntersect<infer Types extends TSchema[]> ? TInferIntersect<ModuleProperties, Types> : Type extends TIterator<infer Type extends TSchema> ? TInferIterator<ModuleProperties, Type> : Type extends TObject<infer Properties extends TProperties> ? TInferObject<ModuleProperties, Properties> : Type extends TRecord<infer Key extends TSchema, infer Type extends TSchema> ? TInferRecord<ModuleProperties, Key, Type> : Type extends TRef<infer Ref extends string> ? TInferRef<ModuleProperties, Ref> : Type extends TTuple<infer Types extends TSchema[]> ? TInferTuple<ModuleProperties, Types> : Type extends TEnum<infer _ extends TEnumRecord> ? Static<Type> : Type extends TUnion<infer Types extends TSchema[]> ? TInferUnion<ModuleProperties, Types> : Type extends TRecursive<infer Schema extends TSchema> ? TInfer<ModuleProperties, Schema> : Static<Type>);
/** Inference Path for Imports. This type is used to compute TImport `static` */
export type TInferFromModuleKey<ModuleProperties extends TProperties, Key extends PropertyKey> = (Key extends keyof ModuleProperties ? TInfer<ModuleProperties, ModuleProperties[Key]> : never);
export {};
