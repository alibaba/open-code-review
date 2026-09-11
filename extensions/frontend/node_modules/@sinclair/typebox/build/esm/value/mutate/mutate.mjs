import { IsObject, IsArray, IsTypedArray, IsValueType } from '../guard/index.mjs';
import { ValuePointer } from '../pointer/index.mjs';
import { Clone } from '../clone/index.mjs';
import { TypeBoxError } from '../../type/error/index.mjs';
// ------------------------------------------------------------------
// IsStandardObject
// ------------------------------------------------------------------
function IsStandardObject(value) {
    return IsObject(value) && !IsArray(value);
}
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
export class ValueMutateError extends TypeBoxError {
    constructor(message) {
        super(message);
    }
}
function ObjectType(root, path, current, next) {
    if (!IsStandardObject(current)) {
        ValuePointer.Set(root, path, Clone(next));
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
    if (!IsArray(current)) {
        ValuePointer.Set(root, path, Clone(next));
    }
    else {
        for (let index = 0; index < next.length; index++) {
            Visit(root, `${path}/${index}`, current[index], next[index]);
        }
        current.splice(next.length);
    }
}
function TypedArrayType(root, path, current, next) {
    if (IsTypedArray(current) && current.length === next.length) {
        for (let i = 0; i < current.length; i++) {
            current[i] = next[i];
        }
    }
    else {
        ValuePointer.Set(root, path, Clone(next));
    }
}
function ValueType(root, path, current, next) {
    if (current === next)
        return;
    ValuePointer.Set(root, path, next);
}
function Visit(root, path, current, next) {
    if (IsArray(next))
        return ArrayType(root, path, current, next);
    if (IsTypedArray(next))
        return TypedArrayType(root, path, current, next);
    if (IsStandardObject(next))
        return ObjectType(root, path, current, next);
    if (IsValueType(next))
        return ValueType(root, path, current, next);
}
// ------------------------------------------------------------------
// IsNonMutableValue
// ------------------------------------------------------------------
function IsNonMutableValue(value) {
    return IsTypedArray(value) || IsValueType(value);
}
function IsMismatchedValue(current, next) {
    // prettier-ignore
    return ((IsStandardObject(current) && IsArray(next)) ||
        (IsArray(current) && IsStandardObject(next)));
}
// ------------------------------------------------------------------
// Mutate
// ------------------------------------------------------------------
/** `[Mutable]` Performs a deep mutable value assignment while retaining internal references */
export function Mutate(current, next) {
    if (IsNonMutableValue(current) || IsNonMutableValue(next))
        throw new ValueMutateError('Only object and array types can be mutated at the root level');
    if (IsMismatchedValue(current, next))
        throw new ValueMutateError('Cannot assign due type mismatch of assignable values');
    Visit(current, '', current, next);
}
