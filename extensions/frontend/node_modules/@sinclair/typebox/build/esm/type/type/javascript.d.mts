import { JsonTypeBuilder } from './json.mjs';
import { type TArgument } from '../argument/index.mjs';
import { type TAsyncIterator } from '../async-iterator/index.mjs';
import { type TAwaited } from '../awaited/index.mjs';
import { type TBigInt, type BigIntOptions } from '../bigint/index.mjs';
import { type TConstructor } from '../constructor/index.mjs';
import { type TConstructorParameters } from '../constructor-parameters/index.mjs';
import { type TDate, type DateOptions } from '../date/index.mjs';
import { type TFunction } from '../function/index.mjs';
import { type TInstanceType } from '../instance-type/index.mjs';
import { type TInstantiate } from '../instantiate/index.mjs';
import { type TIterator } from '../iterator/index.mjs';
import { type TParameters } from '../parameters/index.mjs';
import { type TPromise } from '../promise/index.mjs';
import { type TRegExp, RegExpOptions } from '../regexp/index.mjs';
import { type TReturnType } from '../return-type/index.mjs';
import { type TSchema, type SchemaOptions } from '../schema/index.mjs';
import { type TSymbol } from '../symbol/index.mjs';
import { type TUint8Array, type Uint8ArrayOptions } from '../uint8array/index.mjs';
import { type TUndefined } from '../undefined/index.mjs';
import { type TVoid } from '../void/index.mjs';
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
