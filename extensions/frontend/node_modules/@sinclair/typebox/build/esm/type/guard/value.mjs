// --------------------------------------------------------------------------
// PropertyKey
// --------------------------------------------------------------------------
/** Returns true if this value has this property key */
export function HasPropertyKey(value, key) {
    return key in value;
}
// --------------------------------------------------------------------------
// Object Instances
// --------------------------------------------------------------------------
/** Returns true if this value is an async iterator */
export function IsAsyncIterator(value) {
    return IsObject(value) && !IsArray(value) && !IsUint8Array(value) && Symbol.asyncIterator in value;
}
/** Returns true if this value is an array */
export function IsArray(value) {
    return Array.isArray(value);
}
/** Returns true if this value is bigint */
export function IsBigInt(value) {
    return typeof value === 'bigint';
}
/** Returns true if this value is a boolean */
export function IsBoolean(value) {
    return typeof value === 'boolean';
}
/** Returns true if this value is a Date object */
export function IsDate(value) {
    return value instanceof globalThis.Date;
}
/** Returns true if this value is a function */
export function IsFunction(value) {
    return typeof value === 'function';
}
/** Returns true if this value is an iterator */
export function IsIterator(value) {
    return IsObject(value) && !IsArray(value) && !IsUint8Array(value) && Symbol.iterator in value;
}
/** Returns true if this value is null */
export function IsNull(value) {
    return value === null;
}
/** Returns true if this value is number */
export function IsNumber(value) {
    return typeof value === 'number';
}
/** Returns true if this value is an object */
export function IsObject(value) {
    return typeof value === 'object' && value !== null;
}
/** Returns true if this value is RegExp */
export function IsRegExp(value) {
    return value instanceof globalThis.RegExp;
}
/** Returns true if this value is string */
export function IsString(value) {
    return typeof value === 'string';
}
/** Returns true if this value is symbol */
export function IsSymbol(value) {
    return typeof value === 'symbol';
}
/** Returns true if this value is a Uint8Array */
export function IsUint8Array(value) {
    return value instanceof globalThis.Uint8Array;
}
/** Returns true if this value is undefined */
export function IsUndefined(value) {
    return value === undefined;
}
