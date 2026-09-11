import * as ValueGuard from '../guard/value.mjs';
function ImmutableArray(value) {
    return globalThis.Object.freeze(value).map((value) => Immutable(value));
}
function ImmutableDate(value) {
    return value;
}
function ImmutableUint8Array(value) {
    return value;
}
function ImmutableRegExp(value) {
    return value;
}
function ImmutableObject(value) {
    const result = {};
    for (const key of Object.getOwnPropertyNames(value)) {
        result[key] = Immutable(value[key]);
    }
    for (const key of Object.getOwnPropertySymbols(value)) {
        result[key] = Immutable(value[key]);
    }
    return globalThis.Object.freeze(result);
}
/** Specialized deep immutable value. Applies freeze recursively to the given value */
// prettier-ignore
export function Immutable(value) {
    return (ValueGuard.IsArray(value) ? ImmutableArray(value) :
        ValueGuard.IsDate(value) ? ImmutableDate(value) :
            ValueGuard.IsUint8Array(value) ? ImmutableUint8Array(value) :
                ValueGuard.IsRegExp(value) ? ImmutableRegExp(value) :
                    ValueGuard.IsObject(value) ? ImmutableObject(value) :
                        value);
}
