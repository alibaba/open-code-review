"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.HasPropertyKey = HasPropertyKey;
exports.IsAsyncIterator = IsAsyncIterator;
exports.IsArray = IsArray;
exports.IsBigInt = IsBigInt;
exports.IsBoolean = IsBoolean;
exports.IsDate = IsDate;
exports.IsFunction = IsFunction;
exports.IsIterator = IsIterator;
exports.IsNull = IsNull;
exports.IsNumber = IsNumber;
exports.IsObject = IsObject;
exports.IsRegExp = IsRegExp;
exports.IsString = IsString;
exports.IsSymbol = IsSymbol;
exports.IsUint8Array = IsUint8Array;
exports.IsUndefined = IsUndefined;
// --------------------------------------------------------------------------
// PropertyKey
// --------------------------------------------------------------------------
/** Returns true if this value has this property key */
function HasPropertyKey(value, key) {
    return key in value;
}
// --------------------------------------------------------------------------
// Object Instances
// --------------------------------------------------------------------------
/** Returns true if this value is an async iterator */
function IsAsyncIterator(value) {
    return IsObject(value) && !IsArray(value) && !IsUint8Array(value) && Symbol.asyncIterator in value;
}
/** Returns true if this value is an array */
function IsArray(value) {
    return Array.isArray(value);
}
/** Returns true if this value is bigint */
function IsBigInt(value) {
    return typeof value === 'bigint';
}
/** Returns true if this value is a boolean */
function IsBoolean(value) {
    return typeof value === 'boolean';
}
/** Returns true if this value is a Date object */
function IsDate(value) {
    return value instanceof globalThis.Date;
}
/** Returns true if this value is a function */
function IsFunction(value) {
    return typeof value === 'function';
}
/** Returns true if this value is an iterator */
function IsIterator(value) {
    return IsObject(value) && !IsArray(value) && !IsUint8Array(value) && Symbol.iterator in value;
}
/** Returns true if this value is null */
function IsNull(value) {
    return value === null;
}
/** Returns true if this value is number */
function IsNumber(value) {
    return typeof value === 'number';
}
/** Returns true if this value is an object */
function IsObject(value) {
    return typeof value === 'object' && value !== null;
}
/** Returns true if this value is RegExp */
function IsRegExp(value) {
    return value instanceof globalThis.RegExp;
}
/** Returns true if this value is string */
function IsString(value) {
    return typeof value === 'string';
}
/** Returns true if this value is symbol */
function IsSymbol(value) {
    return typeof value === 'symbol';
}
/** Returns true if this value is a Uint8Array */
function IsUint8Array(value) {
    return value instanceof globalThis.Uint8Array;
}
/** Returns true if this value is undefined */
function IsUndefined(value) {
    return value === undefined;
}
