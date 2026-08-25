import { HasPropertyKey, IsStandardObject, IsArray, IsTypedArray, IsValueType } from '../guard/index.mjs';
import { ValuePointer } from '../pointer/index.mjs';
import { Clone } from '../clone/index.mjs';
import { Equal } from '../equal/equal.mjs';
import { TypeBoxError } from '../../type/error/index.mjs';
import { Literal } from '../../type/literal/index.mjs';
import { Object as _Object_ } from '../../type/object/index.mjs';
import { String } from '../../type/string/index.mjs';
import { Unknown } from '../../type/unknown/index.mjs';
import { Union } from '../../type/union/index.mjs';
export const Insert = _Object_({
    type: Literal('insert'),
    path: String(),
    value: Unknown(),
});
export const Update = _Object_({
    type: Literal('update'),
    path: String(),
    value: Unknown(),
});
export const Delete = _Object_({
    type: Literal('delete'),
    path: String(),
});
export const Edit = Union([Insert, Update, Delete]);
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
export class ValueDiffError extends TypeBoxError {
    constructor(value, message) {
        super(message);
        this.value = value;
    }
}
// ------------------------------------------------------------------
// Command Factory
// ------------------------------------------------------------------
function CreateUpdate(path, value) {
    return { type: 'update', path, value };
}
function CreateInsert(path, value) {
    return { type: 'insert', path, value };
}
function CreateDelete(path) {
    return { type: 'delete', path };
}
// ------------------------------------------------------------------
// AssertDiffable
// ------------------------------------------------------------------
function AssertDiffable(value) {
    if (globalThis.Object.getOwnPropertySymbols(value).length > 0)
        throw new ValueDiffError(value, 'Cannot diff objects with symbols');
}
// ------------------------------------------------------------------
// Diffing Generators
// ------------------------------------------------------------------
function* ObjectType(path, current, next) {
    AssertDiffable(current);
    AssertDiffable(next);
    if (!IsStandardObject(next))
        return yield CreateUpdate(path, next);
    const currentKeys = globalThis.Object.getOwnPropertyNames(current);
    const nextKeys = globalThis.Object.getOwnPropertyNames(next);
    // ----------------------------------------------------------------
    // inserts
    // ----------------------------------------------------------------
    for (const key of nextKeys) {
        if (HasPropertyKey(current, key))
            continue;
        yield CreateInsert(`${path}/${key}`, next[key]);
    }
    // ----------------------------------------------------------------
    // updates
    // ----------------------------------------------------------------
    for (const key of currentKeys) {
        if (!HasPropertyKey(next, key))
            continue;
        if (Equal(current, next))
            continue;
        yield* Visit(`${path}/${key}`, current[key], next[key]);
    }
    // ----------------------------------------------------------------
    // deletes
    // ----------------------------------------------------------------
    for (const key of currentKeys) {
        if (HasPropertyKey(next, key))
            continue;
        yield CreateDelete(`${path}/${key}`);
    }
}
function* ArrayType(path, current, next) {
    if (!IsArray(next))
        return yield CreateUpdate(path, next);
    for (let i = 0; i < Math.min(current.length, next.length); i++) {
        yield* Visit(`${path}/${i}`, current[i], next[i]);
    }
    for (let i = 0; i < next.length; i++) {
        if (i < current.length)
            continue;
        yield CreateInsert(`${path}/${i}`, next[i]);
    }
    for (let i = current.length - 1; i >= 0; i--) {
        if (i < next.length)
            continue;
        yield CreateDelete(`${path}/${i}`);
    }
}
function* TypedArrayType(path, current, next) {
    if (!IsTypedArray(next) || current.length !== next.length || globalThis.Object.getPrototypeOf(current).constructor.name !== globalThis.Object.getPrototypeOf(next).constructor.name)
        return yield CreateUpdate(path, next);
    for (let i = 0; i < Math.min(current.length, next.length); i++) {
        yield* Visit(`${path}/${i}`, current[i], next[i]);
    }
}
function* ValueType(path, current, next) {
    if (current === next)
        return;
    yield CreateUpdate(path, next);
}
function* Visit(path, current, next) {
    if (IsStandardObject(current))
        return yield* ObjectType(path, current, next);
    if (IsArray(current))
        return yield* ArrayType(path, current, next);
    if (IsTypedArray(current))
        return yield* TypedArrayType(path, current, next);
    if (IsValueType(current))
        return yield* ValueType(path, current, next);
    throw new ValueDiffError(current, 'Unable to diff value');
}
// ------------------------------------------------------------------
// Diff
// ------------------------------------------------------------------
export function Diff(current, next) {
    return [...Visit('', current, next)];
}
// ------------------------------------------------------------------
// Patch
// ------------------------------------------------------------------
function IsRootUpdate(edits) {
    return edits.length > 0 && edits[0].path === '' && edits[0].type === 'update';
}
function IsIdentity(edits) {
    return edits.length === 0;
}
export function Patch(current, edits) {
    if (IsRootUpdate(edits)) {
        return Clone(edits[0].value);
    }
    if (IsIdentity(edits)) {
        return Clone(current);
    }
    const clone = Clone(current);
    for (const edit of edits) {
        switch (edit.type) {
            case 'insert': {
                ValuePointer.Set(clone, edit.path, edit.value);
                break;
            }
            case 'update': {
                ValuePointer.Set(clone, edit.path, edit.value);
                break;
            }
            case 'delete': {
                ValuePointer.Delete(clone, edit.path);
                break;
            }
        }
    }
    return clone;
}
