import * as ValueGuard from '../guard/value.mjs';
function ArrayType(value) {
    return value.map((value) => Visit(value));
}
function DateType(value) {
    return new Date(value.getTime());
}
function Uint8ArrayType(value) {
    return new Uint8Array(value);
}
function RegExpType(value) {
    return new RegExp(value.source, value.flags);
}
function ObjectType(value) {
    const result = {};
    for (const key of Object.getOwnPropertyNames(value)) {
        result[key] = Visit(value[key]);
    }
    for (const key of Object.getOwnPropertySymbols(value)) {
        result[key] = Visit(value[key]);
    }
    return result;
}
// prettier-ignore
function Visit(value) {
    return (ValueGuard.IsArray(value) ? ArrayType(value) :
        ValueGuard.IsDate(value) ? DateType(value) :
            ValueGuard.IsUint8Array(value) ? Uint8ArrayType(value) :
                ValueGuard.IsRegExp(value) ? RegExpType(value) :
                    ValueGuard.IsObject(value) ? ObjectType(value) :
                        value);
}
/** Clones a value */
export function Clone(value) {
    return Visit(value);
}
