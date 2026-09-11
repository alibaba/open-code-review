/** Returns true if this value has this property key */
export declare function HasPropertyKey<K extends PropertyKey>(value: Record<any, unknown>, key: K): value is Record<PropertyKey, unknown> & {
    [_ in K]: unknown;
};
/** Returns true if this value is an async iterator */
export declare function IsAsyncIterator(value: unknown): value is AsyncIterableIterator<unknown>;
/** Returns true if this value is an array */
export declare function IsArray(value: unknown): value is unknown[];
/** Returns true if this value is bigint */
export declare function IsBigInt(value: unknown): value is bigint;
/** Returns true if this value is a boolean */
export declare function IsBoolean(value: unknown): value is boolean;
/** Returns true if this value is a Date object */
export declare function IsDate(value: unknown): value is Date;
/** Returns true if this value is a function */
export declare function IsFunction(value: unknown): value is Function;
/** Returns true if this value is an iterator */
export declare function IsIterator(value: unknown): value is IterableIterator<unknown>;
/** Returns true if this value is null */
export declare function IsNull(value: unknown): value is null;
/** Returns true if this value is number */
export declare function IsNumber(value: unknown): value is number;
/** Returns true if this value is an object */
export declare function IsObject(value: unknown): value is Record<PropertyKey, unknown>;
/** Returns true if this value is RegExp */
export declare function IsRegExp(value: unknown): value is RegExp;
/** Returns true if this value is string */
export declare function IsString(value: unknown): value is string;
/** Returns true if this value is symbol */
export declare function IsSymbol(value: unknown): value is symbol;
/** Returns true if this value is a Uint8Array */
export declare function IsUint8Array(value: unknown): value is Uint8Array;
/** Returns true if this value is undefined */
export declare function IsUndefined(value: unknown): value is undefined;
