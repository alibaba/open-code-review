"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Clone = Clone;
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const index_1 = require("../guard/index");
// ------------------------------------------------------------------
// Clonable
// ------------------------------------------------------------------
function FromObject(value) {
    const Acc = {};
    for (const key of Object.getOwnPropertyNames(value)) {
        Acc[key] = Clone(value[key]);
    }
    for (const key of Object.getOwnPropertySymbols(value)) {
        Acc[key] = Clone(value[key]);
    }
    return Acc;
}
function FromArray(value) {
    return value.map((element) => Clone(element));
}
function FromTypedArray(value) {
    return value.slice();
}
function FromMap(value) {
    return new Map(Clone([...value.entries()]));
}
function FromSet(value) {
    return new Set(Clone([...value.entries()]));
}
function FromDate(value) {
    return new Date(value.toISOString());
}
function FromValue(value) {
    return value;
}
// ------------------------------------------------------------------
// Clone
// ------------------------------------------------------------------
/** Returns a clone of the given value */
function Clone(value) {
    if ((0, index_1.IsArray)(value))
        return FromArray(value);
    if ((0, index_1.IsDate)(value))
        return FromDate(value);
    if ((0, index_1.IsTypedArray)(value))
        return FromTypedArray(value);
    if ((0, index_1.IsMap)(value))
        return FromMap(value);
    if ((0, index_1.IsSet)(value))
        return FromSet(value);
    if ((0, index_1.IsObject)(value))
        return FromObject(value);
    if ((0, index_1.IsValueType)(value))
        return FromValue(value);
    throw new Error('ValueClone: Unable to clone value');
}
