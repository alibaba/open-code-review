export type ObjectType = Record<PropertyKey, unknown>;
export type ArrayType = unknown[];
export type ValueType = null | undefined | symbol | bigint | number | boolean | string;
export type TypedArrayType = Int8Array | Uint8Array | Uint8ClampedArray | Int16Array | Uint16Array | Int32Array | Uint32Array | Float32Array | Float64Array | BigInt64Array | BigUint64Array;
/** Returns true if this value is an async iterator */
export declare function IsAsyncIterator(value: unknown): value is AsyncIterableIterator<any>;
/** Returns true if this value is an iterator */
export declare function IsIterator(value: unknown): value is IterableIterator<any>;
/** Returns true if this value is not an instance of a class */
export declare function IsStandardObject(value: unknown): value is ObjectType;
/** Returns true if this value is an instance of a class */
export declare function IsInstanceObject(value: unknown): value is ObjectType;
/** Returns true if this value is a Promise */
export declare function IsPromise(value: unknown): value is Promise<unknown>;
/** Returns true if this value is a Date */
export declare function IsDate(value: unknown): value is Date;
/** Returns true if this value is an instance of Map<K, T> */
export declare function IsMap(value: unknown): value is Map<unknown, unknown>;
/** Returns true if this value is an instance of Set<T> */
export declare function IsSet(value: unknown): value is Set<unknown>;
/** Returns true if this value is RegExp */
export declare function IsRegExp(value: unknown): value is RegExp;
/** Returns true if this value is a typed array */
export declare function IsTypedArray(value: unknown): value is TypedArrayType;
/** Returns true if the value is a Int8Array */
export declare function IsInt8Array(value: unknown): value is Int8Array;
/** Returns true if the value is a Uint8Array */
export declare function IsUint8Array(value: unknown): value is Uint8Array;
/** Returns true if the value is a Uint8ClampedArray */
export declare function IsUint8ClampedArray(value: unknown): value is Uint8ClampedArray;
/** Returns true if the value is a Int16Array */
export declare function IsInt16Array(value: unknown): value is Int16Array;
/** Returns true if the value is a Uint16Array */
export declare function IsUint16Array(value: unknown): value is Uint16Array;
/** Returns true if the value is a Int32Array */
export declare function IsInt32Array(value: unknown): value is Int32Array;
/** Returns true if the value is a Uint32Array */
export declare function IsUint32Array(value: unknown): value is Uint32Array;
/** Returns true if the value is a Float32Array */
export declare function IsFloat32Array(value: unknown): value is Float32Array;
/** Returns true if the value is a Float64Array */
export declare function IsFloat64Array(value: unknown): value is Float64Array;
/** Returns true if the value is a BigInt64Array */
export declare function IsBigInt64Array(value: unknown): value is BigInt64Array;
/** Returns true if the value is a BigUint64Array */
export declare function IsBigUint64Array(value: unknown): value is BigUint64Array;
/** Returns true if this value has this property key */
export declare function HasPropertyKey<K extends PropertyKey>(value: Record<any, unknown>, key: K): value is Record<PropertyKey, unknown> & {
    [_ in K]: unknown;
};
/** Returns true of this value is an object type */
export declare function IsObject(value: unknown): value is ObjectType;
/** Returns true if this value is an array, but not a typed array */
export declare function IsArray(value: unknown): value is ArrayType;
/** Returns true if this value is an undefined */
export declare function IsUndefined(value: unknown): value is undefined;
/** Returns true if this value is an null */
export declare function IsNull(value: unknown): value is null;
/** Returns true if this value is an boolean */
export declare function IsBoolean(value: unknown): value is boolean;
/** Returns true if this value is an number */
export declare function IsNumber(value: unknown): value is number;
/** Returns true if this value is an integer */
export declare function IsInteger(value: unknown): value is number;
/** Returns true if this value is bigint */
export declare function IsBigInt(value: unknown): value is bigint;
/** Returns true if this value is string */
export declare function IsString(value: unknown): value is string;
/** Returns true if this value is a function */
export declare function IsFunction(value: unknown): value is Function;
/** Returns true if this value is a symbol */
export declare function IsSymbol(value: unknown): value is symbol;
/** Returns true if this value is a value type such as number, string, boolean */
export declare function IsValueType(value: unknown): value is ValueType;
