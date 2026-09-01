import { IsObject, IsDate, IsArray, IsTypedArray, IsValueType } from '../guard/index.mjs';
// ------------------------------------------------------------------
// Equality Checks
// ------------------------------------------------------------------
function ObjectType(left, right) {
    if (!IsObject(right))
        return false;
    const leftKeys = [...Object.keys(left), ...Object.getOwnPropertySymbols(left)];
    const rightKeys = [...Object.keys(right), ...Object.getOwnPropertySymbols(right)];
    if (leftKeys.length !== rightKeys.length)
        return false;
    return leftKeys.every((key) => Equal(left[key], right[key]));
}
function DateType(left, right) {
    return IsDate(right) && left.getTime() === right.getTime();
}
function ArrayType(left, right) {
    if (!IsArray(right) || left.length !== right.length)
        return false;
    return left.every((value, index) => Equal(value, right[index]));
}
function TypedArrayType(left, right) {
    if (!IsTypedArray(right) || left.length !== right.length || Object.getPrototypeOf(left).constructor.name !== Object.getPrototypeOf(right).constructor.name)
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
export function Equal(left, right) {
    if (IsDate(left))
        return DateType(left, right);
    if (IsTypedArray(left))
        return TypedArrayType(left, right);
    if (IsArray(left))
        return ArrayType(left, right);
    if (IsObject(left))
        return ObjectType(left, right);
    if (IsValueType(left))
        return ValueType(left, right);
    throw new Error('ValueEquals: Unable to compare value');
}
