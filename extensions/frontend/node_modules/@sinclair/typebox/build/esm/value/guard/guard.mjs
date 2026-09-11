// --------------------------------------------------------------------------
// Iterators
// --------------------------------------------------------------------------
/** Returns true if this value is an async iterator */
export function IsAsyncIterator(value) {
    return IsObject(value) && globalThis.Symbol.asyncIterator in value;
}
/** Returns true if this value is an iterator */
export function IsIterator(value) {
    return IsObject(value) && globalThis.Symbol.iterator in value;
}
// --------------------------------------------------------------------------
// Object Instances
// --------------------------------------------------------------------------
/** Returns true if this value is not an instance of a class */
export function IsStandardObject(value) {
    return IsObject(value) && (globalThis.Object.getPrototypeOf(value) === Object.prototype || globalThis.Object.getPrototypeOf(value) === null);
}
/** Returns true if this value is an instance of a class */
export function IsInstanceObject(value) {
    return IsObject(value) && !IsArray(value) && IsFunction(value.constructor) && value.constructor.name !== 'Object';
}
// --------------------------------------------------------------------------
// JavaScript
// --------------------------------------------------------------------------
/** Returns true if this value is a Promise */
export function IsPromise(value) {
    return value instanceof globalThis.Promise;
}
/** Returns true if this value is a Date */
export function IsDate(value) {
    return value instanceof Date && globalThis.Number.isFinite(value.getTime());
}
/** Returns true if this value is an instance of Map<K, T> */
export function IsMap(value) {
    return value instanceof globalThis.Map;
}
/** Returns true if this value is an instance of Set<T> */
export function IsSet(value) {
    return value instanceof globalThis.Set;
}
/** Returns true if this value is RegExp */
export function IsRegExp(value) {
    return value instanceof globalThis.RegExp;
}
/** Returns true if this value is a typed array */
export function IsTypedArray(value) {
    return globalThis.ArrayBuffer.isView(value);
}
/** Returns true if the value is a Int8Array */
export function IsInt8Array(value) {
    return value instanceof globalThis.Int8Array;
}
/** Returns true if the value is a Uint8Array */
export function IsUint8Array(value) {
    return value instanceof globalThis.Uint8Array;
}
/** Returns true if the value is a Uint8ClampedArray */
export function IsUint8ClampedArray(value) {
    return value instanceof globalThis.Uint8ClampedArray;
}
/** Returns true if the value is a Int16Array */
export function IsInt16Array(value) {
    return value instanceof globalThis.Int16Array;
}
/** Returns true if the value is a Uint16Array */
export function IsUint16Array(value) {
    return value instanceof globalThis.Uint16Array;
}
/** Returns true if the value is a Int32Array */
export function IsInt32Array(value) {
    return value instanceof globalThis.Int32Array;
}
/** Returns true if the value is a Uint32Array */
export function IsUint32Array(value) {
    return value instanceof globalThis.Uint32Array;
}
/** Returns true if the value is a Float32Array */
export function IsFloat32Array(value) {
    return value instanceof globalThis.Float32Array;
}
/** Returns true if the value is a Float64Array */
export function IsFloat64Array(value) {
    return value instanceof globalThis.Float64Array;
}
/** Returns true if the value is a BigInt64Array */
export function IsBigInt64Array(value) {
    return value instanceof globalThis.BigInt64Array;
}
/** Returns true if the value is a BigUint64Array */
export function IsBigUint64Array(value) {
    return value instanceof globalThis.BigUint64Array;
}
// --------------------------------------------------------------------------
// PropertyKey
// --------------------------------------------------------------------------
/** Returns true if this value has this property key */
export function HasPropertyKey(value, key) {
    return key in value;
}
// --------------------------------------------------------------------------
// Standard
// --------------------------------------------------------------------------
/** Returns true of this value is an object type */
export function IsObject(value) {
    return value !== null && typeof value === 'object';
}
/** Returns true if this value is an array, but not a typed array */
export function IsArray(value) {
    return globalThis.Array.isArray(value) && !globalThis.ArrayBuffer.isView(value);
}
/** Returns true if this value is an undefined */
export function IsUndefined(value) {
    return value === undefined;
}
/** Returns true if this value is an null */
export function IsNull(value) {
    return value === null;
}
/** Returns true if this value is an boolean */
export function IsBoolean(value) {
    return typeof value === 'boolean';
}
/** Returns true if this value is an number */
export function IsNumber(value) {
    return typeof value === 'number';
}
/** Returns true if this value is an integer */
export function IsInteger(value) {
    return globalThis.Number.isInteger(value);
}
/** Returns true if this value is bigint */
export function IsBigInt(value) {
    return typeof value === 'bigint';
}
/** Returns true if this value is string */
export function IsString(value) {
    return typeof value === 'string';
}
/** Returns true if this value is a function */
export function IsFunction(value) {
    return typeof value === 'function';
}
/** Returns true if this value is a symbol */
export function IsSymbol(value) {
    return typeof value === 'symbol';
}
/** Returns true if this value is a value type such as number, string, boolean */
export function IsValueType(value) {
    // prettier-ignore
    return (IsBigInt(value) ||
        IsBoolean(value) ||
        IsNull(value) ||
        IsNumber(value) ||
        IsString(value) ||
        IsSymbol(value) ||
        IsUndefined(value));
}
