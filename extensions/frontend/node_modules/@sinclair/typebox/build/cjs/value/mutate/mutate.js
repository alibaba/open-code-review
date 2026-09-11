"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ValueMutateError = void 0;
exports.Mutate = Mutate;
const index_1 = require("../guard/index");
const index_2 = require("../pointer/index");
const index_3 = require("../clone/index");
const index_4 = require("../../type/error/index");
// ------------------------------------------------------------------
// IsStandardObject
// ------------------------------------------------------------------
function IsStandardObject(value) {
    return (0, index_1.IsObject)(value) && !(0, index_1.IsArray)(value);
}
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
class ValueMutateError extends index_4.TypeBoxError {
    constructor(message) {
        super(message);
    }
}
exports.ValueMutateError = ValueMutateError;
function ObjectType(root, path, current, next) {
    if (!IsStandardObject(current)) {
        index_2.ValuePointer.Set(root, path, (0, index_3.Clone)(next));
    }
    else {
        const currentKeys = Object.getOwnPropertyNames(current);
        const nextKeys = Object.getOwnPropertyNames(next);
        for (const currentKey of currentKeys) {
            if (!nextKeys.includes(currentKey)) {
                delete current[currentKey];
            }
        }
        for (const nextKey of nextKeys) {
            if (!currentKeys.includes(nextKey)) {
                current[nextKey] = null;
            }
        }
        for (const nextKey of nextKeys) {
            Visit(root, `${path}/${nextKey}`, current[nextKey], next[nextKey]);
        }
    }
}
function ArrayType(root, path, current, next) {
    if (!(0, index_1.IsArray)(current)) {
        index_2.ValuePointer.Set(root, path, (0, index_3.Clone)(next));
    }
    else {
        for (let index = 0; index < next.length; index++) {
            Visit(root, `${path}/${index}`, current[index], next[index]);
        }
        current.splice(next.length);
    }
}
function TypedArrayType(root, path, current, next) {
    if ((0, index_1.IsTypedArray)(current) && current.length === next.length) {
        for (let i = 0; i < current.length; i++) {
            current[i] = next[i];
        }
    }
    else {
        index_2.ValuePointer.Set(root, path, (0, index_3.Clone)(next));
    }
}
function ValueType(root, path, current, next) {
    if (current === next)
        return;
    index_2.ValuePointer.Set(root, path, next);
}
function Visit(root, path, current, next) {
    if ((0, index_1.IsArray)(next))
        return ArrayType(root, path, current, next);
    if ((0, index_1.IsTypedArray)(next))
        return TypedArrayType(root, path, current, next);
    if (IsStandardObject(next))
        return ObjectType(root, path, current, next);
    if ((0, index_1.IsValueType)(next))
        return ValueType(root, path, current, next);
}
// ------------------------------------------------------------------
// IsNonMutableValue
// ------------------------------------------------------------------
function IsNonMutableValue(value) {
    return (0, index_1.IsTypedArray)(value) || (0, index_1.IsValueType)(value);
}
function IsMismatchedValue(current, next) {
    // prettier-ignore
    return ((IsStandardObject(current) && (0, index_1.IsArray)(next)) ||
        ((0, index_1.IsArray)(current) && IsStandardObject(next)));
}
// ------------------------------------------------------------------
// Mutate
// ------------------------------------------------------------------
/** `[Mutable]` Performs a deep mutable value assignment while retaining internal references */
function Mutate(current, next) {
    if (IsNonMutableValue(current) || IsNonMutableValue(next))
        throw new ValueMutateError('Only object and array types can be mutated at the root level');
    if (IsMismatchedValue(current, next))
        throw new ValueMutateError('Cannot assign due type mismatch of assignable values');
    Visit(current, '', current, next);
}
