import { JsonTypeBuilder } from './json';
import { type TArgument } from '../argument/index';
import { type TAsyncIterator } from '../async-iterator/index';
import { type TAwaited } from '../awaited/index';
import { type TBigInt, type BigIntOptions } from '../bigint/index';
import { type TConstructor } from '../constructor/index';
import { type TConstructorParameters } from '../constructor-parameters/index';
import { type TDate, type DateOptions } from '../date/index';
import { type TFunction } from '../function/index';
import { type TInstanceType } from '../instance-type/index';
import { type TInstantiate } from '../instantiate/index';
import { type TIterator } from '../iterator/index';
import { type TParameters } from '../parameters/index';
import { type TPromise } from '../promise/index';
import { type TRegExp, RegExpOptions } from '../regexp/index';
import { type TReturnType } from '../return-type/index';
import { type TSchema, type SchemaOptions } from '../schema/index';
import { type TSymbol } from '../symbol/index';
import { type TUint8Array, type Uint8ArrayOptions } from '../uint8array/index';
import { type TUndefined } from '../undefined/index';
import { type TVoid } from '../void/index';
/** JavaScript Type Builder with Static Resolution for TypeScript */
export declare class JavaScriptTypeBuilder extends JsonTypeBuilder {
    /** `[JavaScript]` Creates a Generic Argument Type */
    Argument<Index extends number>(index: Index): TArgument<Index>;
    /** `[JavaScript]` Creates a AsyncIterator type */
    AsyncIterator<Type extends TSchema>(items: Type, options?: SchemaOptions): TAsyncIterator<Type>;
    /** `[JavaScript]` Constructs a type by recursively unwrapping Promise types */
    Awaited<Type extends TSchema>(schema: Type, options?: SchemaOptions): TAwaited<Type>;
    /** `[JavaScript]` Creates a BigInt type */
    BigInt(options?: BigIntOptions): TBigInt;
    /** `[JavaScript]` Extracts the ConstructorParameters from the given Constructor type */
    ConstructorParameters<Type extends TSchema>(schema: Type, options?: SchemaOptions): TConstructorParameters<Type>;
    /** `[JavaScript]` Creates a Constructor type */
    Constructor<Parameters extends TSchema[], InstanceType extends TSchema>(parameters: [...Parameters], instanceType: InstanceType, options?: SchemaOptions): TConstructor<Parameters, InstanceType>;
    /** `[JavaScript]` Creates a Date type */
    Date(options?: DateOptions): TDate;
    /** `[JavaScript]` Creates a Function type */
    Function<Parameters extends TSchema[], ReturnType extends TSchema>(parameters: [...Parameters], returnType: ReturnType, options?: SchemaOptions): TFunction<Parameters, ReturnType>;
    /** `[JavaScript]` Extracts the InstanceType from the given Constructor type */
    InstanceType<Type extends TSchema>(schema: Type, options?: SchemaOptions): TInstanceType<Type>;
    /** `[JavaScript]` Instantiates a type with the given parameters */
    Instantiate<Type extends TSchema, Parameters extends TSchema[]>(schema: Type, parameters: [...Parameters]): TInstantiate<Type, Parameters>;
    /** `[JavaScript]` Creates an Iterator type */
    Iterator<Type extends TSchema>(items: Type, options?: SchemaOptions): TIterator<Type>;
    /** `[JavaScript]` Extracts the Parameters from the given Function type */
    Parameters<Type extends TSchema>(schema: Type, options?: SchemaOptions): TParameters<Type>;
    /** `[JavaScript]` Creates a Promise type */
    Promise<Type extends TSchema>(item: Type, options?: SchemaOptions): TPromise<Type>;
    /** `[JavaScript]` Creates a RegExp type */
    RegExp(pattern: string, options?: RegExpOptions): TRegExp;
    /** `[JavaScript]` Creates a RegExp type */
    RegExp(regex: RegExp, options?: RegExpOptions): TRegExp;
    /** `[JavaScript]` Extracts the ReturnType from the given Function type */
    ReturnType<Type extends TSchema>(type: Type, options?: SchemaOptions): TReturnType<Type>;
    /** `[JavaScript]` Creates a Symbol type */
    Symbol(options?: SchemaOptions): TSymbol;
    /** `[JavaScript]` Creates a Undefined type */
    Undefined(options?: SchemaOptions): TUndefined;
    /** `[JavaScript]` Creates a Uint8Array type */
    Uint8Array(options?: Uint8ArrayOptions): TUint8Array;
    /** `[JavaScript]` Creates a Void type */
    Void(options?: SchemaOptions): TVoid;
}
