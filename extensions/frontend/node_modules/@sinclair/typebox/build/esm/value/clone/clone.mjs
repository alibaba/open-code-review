// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
import { IsArray, IsDate, IsMap, IsSet, IsObject, IsTypedArray, IsValueType } from '../guard/index.mjs';
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
export function Clone(value) {
    if (IsArray(value))
        return FromArray(value);
    if (IsDate(value))
        return FromDate(value);
    if (IsTypedArray(value))
        return FromTypedArray(value);
    if (IsMap(value))
        return FromMap(value);
    if (IsSet(value))
        return FromSet(value);
    if (IsObject(value))
        return FromObject(value);
    if (IsValueType(value))
        return FromValue(value);
    throw new Error('ValueClone: Unable to clone value');
}
