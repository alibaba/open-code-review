"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ValueDiffError = exports.Edit = exports.Delete = exports.Update = exports.Insert = void 0;
exports.Diff = Diff;
exports.Patch = Patch;
const index_1 = require("../guard/index");
const index_2 = require("../pointer/index");
const index_3 = require("../clone/index");
const equal_1 = require("../equal/equal");
const index_4 = require("../../type/error/index");
const index_5 = require("../../type/literal/index");
const index_6 = require("../../type/object/index");
const index_7 = require("../../type/string/index");
const index_8 = require("../../type/unknown/index");
const index_9 = require("../../type/union/index");
exports.Insert = (0, index_6.Object)({
    type: (0, index_5.Literal)('insert'),
    path: (0, index_7.String)(),
    value: (0, index_8.Unknown)(),
});
exports.Update = (0, index_6.Object)({
    type: (0, index_5.Literal)('update'),
    path: (0, index_7.String)(),
    value: (0, index_8.Unknown)(),
});
exports.Delete = (0, index_6.Object)({
    type: (0, index_5.Literal)('delete'),
    path: (0, index_7.String)(),
});
exports.Edit = (0, index_9.Union)([exports.Insert, exports.Update, exports.Delete]);
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
class ValueDiffError extends index_4.TypeBoxError {
    constructor(value, message) {
        super(message);
        this.value = value;
    }
}
exports.ValueDiffError = ValueDiffError;
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
    if (!(0, index_1.IsStandardObject)(next))
        return yield CreateUpdate(path, next);
    const currentKeys = globalThis.Object.getOwnPropertyNames(current);
    const nextKeys = globalThis.Object.getOwnPropertyNames(next);
    // ----------------------------------------------------------------
    // inserts
    // ----------------------------------------------------------------
    for (const key of nextKeys) {
        if ((0, index_1.HasPropertyKey)(current, key))
            continue;
        yield CreateInsert(`${path}/${key}`, next[key]);
    }
    // ----------------------------------------------------------------
    // updates
    // ----------------------------------------------------------------
    for (const key of currentKeys) {
        if (!(0, index_1.HasPropertyKey)(next, key))
            continue;
        if ((0, equal_1.Equal)(current, next))
            continue;
        yield* Visit(`${path}/${key}`, current[key], next[key]);
    }
    // ----------------------------------------------------------------
    // deletes
    // ----------------------------------------------------------------
    for (const key of currentKeys) {
        if ((0, index_1.HasPropertyKey)(next, key))
            continue;
        yield CreateDelete(`${path}/${key}`);
    }
}
function* ArrayType(path, current, next) {
    if (!(0, index_1.IsArray)(next))
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
    if (!(0, index_1.IsTypedArray)(next) || current.length !== next.length || globalThis.Object.getPrototypeOf(current).constructor.name !== globalThis.Object.getPrototypeOf(next).constructor.name)
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
    if ((0, index_1.IsStandardObject)(current))
        return yield* ObjectType(path, current, next);
    if ((0, index_1.IsArray)(current))
        return yield* ArrayType(path, current, next);
    if ((0, index_1.IsTypedArray)(current))
        return yield* TypedArrayType(path, current, next);
    if ((0, index_1.IsValueType)(current))
        return yield* ValueType(path, current, next);
    throw new ValueDiffError(current, 'Unable to diff value');
}
// ------------------------------------------------------------------
// Diff
// ------------------------------------------------------------------
function Diff(current, next) {
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
function Patch(current, edits) {
    if (IsRootUpdate(edits)) {
        return (0, index_3.Clone)(edits[0].value);
    }
    if (IsIdentity(edits)) {
        return (0, index_3.Clone)(current);
    }
    const clone = (0, index_3.Clone)(current);
    for (const edit of edits) {
        switch (edit.type) {
            case 'insert': {
                index_2.ValuePointer.Set(clone, edit.path, edit.value);
                break;
            }
            case 'update': {
                index_2.ValuePointer.Set(clone, edit.path, edit.value);
                break;
            }
            case 'delete': {
                index_2.ValuePointer.Delete(clone, edit.path);
                break;
            }
        }
    }
    return clone;
}
