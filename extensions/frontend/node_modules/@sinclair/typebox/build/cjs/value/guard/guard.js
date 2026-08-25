"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IsAsyncIterator = IsAsyncIterator;
exports.IsIterator = IsIterator;
exports.IsStandardObject = IsStandardObject;
exports.IsInstanceObject = IsInstanceObject;
exports.IsPromise = IsPromise;
exports.IsDate = IsDate;
exports.IsMap = IsMap;
exports.IsSet = IsSet;
exports.IsRegExp = IsRegExp;
exports.IsTypedArray = IsTypedArray;
exports.IsInt8Array = IsInt8Array;
exports.IsUint8Array = IsUint8Array;
exports.IsUint8ClampedArray = IsUint8ClampedArray;
exports.IsInt16Array = IsInt16Array;
exports.IsUint16Array = IsUint16Array;
exports.IsInt32Array = IsInt32Array;
exports.IsUint32Array = IsUint32Array;
exports.IsFloat32Array = IsFloat32Array;
exports.IsFloat64Array = IsFloat64Array;
exports.IsBigInt64Array = IsBigInt64Array;
exports.IsBigUint64Array = IsBigUint64Array;
exports.HasPropertyKey = HasPropertyKey;
exports.IsObject = IsObject;
exports.IsArray = IsArray;
exports.IsUndefined = IsUndefined;
exports.IsNull = IsNull;
exports.IsBoolean = IsBoolean;
exports.IsNumber = IsNumber;
exports.IsInteger = IsInteger;
exports.IsBigInt = IsBigInt;
exports.IsString = IsString;
exports.IsFunction = IsFunction;
exports.IsSymbol = IsSymbol;
exports.IsValueType = IsValueType;
// --------------------------------------------------------------------------
// Iterators
// --------------------------------------------------------------------------
/** Returns true if this value is an async iterator */
function IsAsyncIterator(value) {
    return IsObject(value) && globalThis.Symbol.asyncIterator in value;
}
/** Returns true if this value is an iterator */
function IsIterator(value) {
    return IsObject(value) && globalThis.Symbol.iterator in value;
}
// --------------------------------------------------------------------------
// Object Instances
// --------------------------------------------------------------------------
/** Returns true if this value is not an instance of a class */
function IsStandardObject(value) {
    return IsObject(value) && (globalThis.Object.getPrototypeOf(value) === Object.prototype || globalThis.Object.getPrototypeOf(value) === null);
}
/** Returns true if this value is an instance of a class */
function IsInstanceObject(value) {
    return IsObject(value) && !IsArray(value) && IsFunction(value.constructor) && value.constructor.name !== 'Object';
}
// --------------------------------------------------------------------------
// JavaScript
// --------------------------------------------------------------------------
/** Returns true if this value is a Promise */
function IsPromise(value) {
    return value instanceof globalThis.Promise;
}
/** Returns true if this value is a Date */
function IsDate(value) {
    return value instanceof Date && globalThis.Number.isFinite(value.getTime());
}
/** Returns true if this value is an instance of Map<K, T> */
function IsMap(value) {
    return value instanceof globalThis.Map;
}
/** Returns true if this value is an instance of Set<T> */
function IsSet(value) {
    return value instanceof globalThis.Set;
}
/** Returns true if this value is RegExp */
function IsRegExp(value) {
    return value instanceof globalThis.RegExp;
}
/** Returns true if this value is a typed array */
function IsTypedArray(value) {
    return globalThis.ArrayBuffer.isView(value);
}
/** Returns true if the value is a Int8Array */
function IsInt8Array(value) {
    return value instanceof globalThis.Int8Array;
}
/** Returns true if the value is a Uint8Array */
function IsUint8Array(value) {
    return value instanceof globalThis.Uint8Array;
}
/** Returns true if the value is a Uint8ClampedArray */
function IsUint8ClampedArray(value) {
    return value instanceof globalThis.Uint8ClampedArray;
}
/** Returns true if the value is a Int16Array */
function IsInt16Array(value) {
    return value instanceof globalThis.Int16Array;
}
/** Returns true if the value is a Uint16Array */
function IsUint16Array(value) {
    return value instanceof globalThis.Uint16Array;
}
/** Returns true if the value is a Int32Array */
function IsInt32Array(value) {
    return value instanceof globalThis.Int32Array;
}
/** Returns true if the value is a Uint32Array */
function IsUint32Array(value) {
    return value instanceof globalThis.Uint32Array;
}
/** Returns true if the value is a Float32Array */
function IsFloat32Array(value) {
    return value instanceof globalThis.Float32Array;
}
/** Returns true if the value is a Float64Array */
function IsFloat64Array(value) {
    return value instanceof globalThis.Float64Array;
}
/** Returns true if the value is a BigInt64Array */
function IsBigInt64Array(value) {
    return value instanceof globalThis.BigInt64Array;
}
/** Returns true if the value is a BigUint64Array */
function IsBigUint64Array(value) {
    return value instanceof globalThis.BigUint64Array;
}
// --------------------------------------------------------------------------
// PropertyKey
// --------------------------------------------------------------------------
/** Returns true if this value has this property key */
function HasPropertyKey(value, key) {
    return key in value;
}
// --------------------------------------------------------------------------
// Standard
// --------------------------------------------------------------------------
/** Returns true of this value is an object type */
function IsObject(value) {
    return value !== null && typeof value === 'object';
}
/** Returns true if this value is an array, but not a typed array */
function IsArray(value) {
    return globalThis.Array.isArray(value) && !globalThis.ArrayBuffer.isView(value);
}
/** Returns true if this value is an undefined */
function IsUndefined(value) {
    return value === undefined;
}
/** Returns true if this value is an null */
function IsNull(value) {
    return value === null;
}
/** Returns true if this value is an boolean */
function IsBoolean(value) {
    return typeof value === 'boolean';
}
/** Returns true if this value is an number */
function IsNumber(value) {
    return typeof value === 'number';
}
/** Returns true if this value is an integer */
function IsInteger(value) {
    return globalThis.Number.isInteger(value);
}
/** Returns true if this value is bigint */
function IsBigInt(value) {
    return typeof value === 'bigint';
}
/** Returns true if this value is string */
function IsString(value) {
    return typeof value === 'string';
}
/** Returns true if this value is a function */
function IsFunction(value) {
    return typeof value === 'function';
}
/** Returns true if this value is a symbol */
function IsSymbol(value) {
    return typeof value === 'symbol';
}
/** Returns true if this value is a value type such as number, string, boolean */
function IsValueType(value) {
    // prettier-ignore
    return (IsBigInt(value) ||
        IsBoolean(value) ||
        IsNull(value) ||
        IsNumber(value) ||
        IsString(value) ||
        IsSymbol(value) ||
        IsUndefined(value));
}
