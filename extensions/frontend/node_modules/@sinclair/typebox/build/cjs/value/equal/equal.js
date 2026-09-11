"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Equal = Equal;
const index_1 = require("../guard/index");
// ------------------------------------------------------------------
// Equality Checks
// ------------------------------------------------------------------
function ObjectType(left, right) {
    if (!(0, index_1.IsObject)(right))
        return false;
    const leftKeys = [...Object.keys(left), ...Object.getOwnPropertySymbols(left)];
    const rightKeys = [...Object.keys(right), ...Object.getOwnPropertySymbols(right)];
    if (leftKeys.length !== rightKeys.length)
        return false;
    return leftKeys.every((key) => Equal(left[key], right[key]));
}
function DateType(left, right) {
    return (0, index_1.IsDate)(right) && left.getTime() === right.getTime();
}
function ArrayType(left, right) {
    if (!(0, index_1.IsArray)(right) || left.length !== right.length)
        return false;
    return left.every((value, index) => Equal(value, right[index]));
}
function TypedArrayType(left, right) {
    if (!(0, index_1.IsTypedArray)(right) || left.length !== right.length || Object.getPrototypeOf(left).constructor.name !== Object.getPrototypeOf(right).constructor.name)
        return false;
    return left.every((value, index) => Equal(value, right[index]));
}
function ValueType(left, right) {
    return left === right;
}
// ------------------------------------------------------------------
// Equal
// ------------------------------------------------------------------
/** Returns true if the left value deep-equals the right */
function Equal(left, right) {
    if ((0, index_1.IsDate)(left))
        return DateType(left, right);
    if ((0, index_1.IsTypedArray)(left))
        return TypedArrayType(left, right);
    if ((0, index_1.IsArray)(left))
        return ArrayType(left, right);
    if ((0, index_1.IsObject)(left))
        return ObjectType(left, right);
    if ((0, index_1.IsValueType)(left))
        return ValueType(left, right);
    throw new Error('ValueEquals: Unable to compare value');
}
