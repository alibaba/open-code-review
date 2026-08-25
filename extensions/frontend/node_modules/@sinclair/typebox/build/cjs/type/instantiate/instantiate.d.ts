import { type TSchema } from '../schema/index';
import { type TArgument } from '../argument/index';
import { type TUnknown } from '../unknown/index';
import { type TReadonlyOptional } from '../readonly-optional/index';
import { type TReadonly } from '../readonly/index';
import { type TOptional } from '../optional/index';
import { type TConstructor } from '../constructor/index';
import { type TFunction } from '../function/index';
import { type TIntersect } from '../intersect/index';
import { type TUnion } from '../union/index';
import { type TTuple } from '../tuple/index';
import { type TArray } from '../array/index';
import { type TAsyncIterator } from '../async-iterator/index';
import { type TIterator } from '../iterator/index';
import { type TPromise } from '../promise/index';
import { type TObject, type TProperties } from '../object/index';
import { type TRecordOrObject, type TRecord } from '../record/index';
type TFromConstructor<Args extends TSchema[], Parameters extends TSchema[], InstanceType extends TSchema, Result extends TConstructor = TConstructor<TFromTypes<Args, Parameters>, TFromType<Args, InstanceType>>> = Result;
type TFromFunction<Args extends TSchema[], Parameters extends TSchema[], ReturnType extends TSchema, Result extends TFunction = TFunction<TFromTypes<Args, Parameters>, TFromType<Args, ReturnType>>> = Result;
type TFromIntersect<Args extends TSchema[], Types extends TSchema[], Result extends TIntersect = TIntersect<TFromTypes<Args, Types>>> = Result;
type TFromUnion<Args extends TSchema[], Types extends TSchema[], Result extends TUnion = TUnion<TFromTypes<Args, Types>>> = Result;
type TFromTuple<Args extends TSchema[], Types extends TSchema[], Result extends TTuple = TTuple<TFromTypes<Args, Types>>> = Result;
type TFromArray<Args extends TSchema[], Type extends TSchema, Result extends TArray = TArray<TFromType<Args, Type>>> = Result;
type TFromAsyncIterator<Args extends TSchema[], Type extends TSchema, Result extends TAsyncIterator = TAsyncIterator<TFromType<Args, Type>>> = Result;
type TFromIterator<Args extends TSchema[], Type extends TSchema, Result extends TIterator = TIterator<TFromType<Args, Type>>> = Result;
type TFromPromise<Args extends TSchema[], Type extends TSchema, Result extends TPromise = TPromise<TFromType<Args, Type>>> = Result;
type TFromObject<Args extends TSchema[], Properties extends TProperties, MappedProperties extends TProperties = TFromProperties<Args, Properties>, Result extends TSchema = TObject<MappedProperties>> = Result;
type TFromRecord<Args extends TSchema[], Key extends TSchema, Value extends TSchema, MappedKey extends TSchema = TFromType<Args, Key>, MappedValue extends TSchema = TFromType<Args, Value>, Result extends TSchema = TRecordOrObject<MappedKey, MappedValue>> = Result;
type TFromArgument<Args extends TSchema[], Index extends number, Result extends TSchema = Index extends keyof Args[Index] ? Args[Index] : TUnknown> = Result;
type TFromProperty<Args extends TSchema[], Type extends TSchema, IsReadonly extends boolean = Type extends TReadonly<Type> ? true : false, IsOptional extends boolean = Type extends TOptional<Type> ? true : false, Mapped extends TSchema = TFromType<Args, Type>, Result extends TSchema = ([
    IsReadonly,
    IsOptional
] extends [true, true] ? TReadonlyOptional<Mapped> : [
    IsReadonly,
    IsOptional
] extends [true, false] ? TReadonly<Mapped> : [
    IsReadonly,
    IsOptional
] extends [false, true] ? TOptional<Mapped> : Mapped)> = Result;
type TFromProperties<Args extends TSchema[], Properties extends TProperties, Result extends TProperties = {
    [Key in keyof Properties]: TFromProperty<Args, Properties[Key]>;
}> = Result;
export type TFromTypes<Args extends TSchema[], Types extends TSchema[], Result extends TSchema[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? TFromTypes<Args, Right, [...Result, TFromType<Args, Left>]> : Result);
export declare function FromTypes<Args extends TSchema[], Types extends TSchema[]>(args: [...Args], types: [...Types]): TFromTypes<Args, Types>;
export type TFromType<Args extends TSchema[], Type extends TSchema> = (Type extends TConstructor<infer Parameters extends TSchema[], infer InstanceType extends TSchema> ? TFromConstructor<Args, Parameters, InstanceType> : Type extends TFunction<infer Parameters extends TSchema[], infer ReturnType extends TSchema> ? TFromFunction<Args, Parameters, ReturnType> : Type extends TIntersect<infer Types extends TSchema[]> ? TFromIntersect<Args, Types> : Type extends TUnion<infer Types extends TSchema[]> ? TFromUnion<Args, Types> : Type extends TTuple<infer Types extends TSchema[]> ? TFromTuple<Args, Types> : Type extends TArray<infer Type extends TSchema> ? TFromArray<Args, Type> : Type extends TAsyncIterator<infer Type extends TSchema> ? TFromAsyncIterator<Args, Type> : Type extends TIterator<infer Type extends TSchema> ? TFromIterator<Args, Type> : Type extends TPromise<infer Type extends TSchema> ? TFromPromise<Args, Type> : Type extends TObject<infer Properties extends TProperties> ? TFromObject<Args, Properties> : Type extends TRecord<infer Key extends TSchema, infer Value extends TSchema> ? TFromRecord<Args, Key, Value> : Type extends TArgument<infer Index extends number> ? TFromArgument<Args, Index> : Type);
/** `[JavaScript]` Instantiates a type with the given parameters */
export type TInstantiate<Type extends TSchema, Args extends TSchema[], Result extends TSchema = TFromType<Args, Type>> = Result;
/** `[JavaScript]` Instantiates a type with the given parameters */
export declare function Instantiate<Type extends TSchema, Args extends TSchema[]>(type: Type, args: [...Args]): TInstantiate<Type, Args>;
export {};
