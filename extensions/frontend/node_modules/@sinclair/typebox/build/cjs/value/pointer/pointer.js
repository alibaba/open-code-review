"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ValuePointerRootDeleteError = exports.ValuePointerRootSetError = void 0;
exports.Format = Format;
exports.Set = Set;
exports.Delete = Delete;
exports.Has = Has;
exports.Get = Get;
const index_1 = require("../../type/error/index");
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
class ValuePointerRootSetError extends index_1.TypeBoxError {
    constructor(value, path, update) {
        super('Cannot set root value');
        this.value = value;
        this.path = path;
        this.update = update;
    }
}
exports.ValuePointerRootSetError = ValuePointerRootSetError;
class ValuePointerRootDeleteError extends index_1.TypeBoxError {
    constructor(value, path) {
        super('Cannot delete root value');
        this.value = value;
        this.path = path;
    }
}
exports.ValuePointerRootDeleteError = ValuePointerRootDeleteError;
// ------------------------------------------------------------------
// ValuePointer
// ------------------------------------------------------------------
/** Provides functionality to update values through RFC6901 string pointers */
// prettier-ignore
function Escape(component) {
    return component.indexOf('~') === -1 ? component : component.replace(/~1/g, '/').replace(/~0/g, '~');
}
/** Formats the given pointer into navigable key components */
// prettier-ignore
function* Format(pointer) {
    if (pointer === '')
        return;
    let [start, end] = [0, 0];
    for (let i = 0; i < pointer.length; i++) {
        const char = pointer.charAt(i);
        if (char === '/') {
            if (i === 0) {
                start = i + 1;
            }
            else {
                end = i;
                yield Escape(pointer.slice(start, end));
                start = i + 1;
            }
        }
        else {
            end = i;
        }
    }
    yield Escape(pointer.slice(start));
}
/** Sets the value at the given pointer. If the value at the pointer does not exist it is created */
// prettier-ignore
function Set(value, pointer, update) {
    if (pointer === '')
        throw new ValuePointerRootSetError(value, pointer, update);
    let [owner, next, key] = [null, value, ''];
    for (const component of Format(pointer)) {
        if (next[component] === undefined)
            next[component] = {};
        owner = next;
        next = next[component];
        key = component;
    }
    owner[key] = update;
}
/** Deletes a value at the given pointer */
// prettier-ignore
function Delete(value, pointer) {
    if (pointer === '')
        throw new ValuePointerRootDeleteError(value, pointer);
    let [owner, next, key] = [null, value, ''];
    for (const component of Format(pointer)) {
        if (next[component] === undefined || next[component] === null)
            return;
        owner = next;
        next = next[component];
        key = component;
    }
    if (Array.isArray(owner)) {
        const index = parseInt(key);
        owner.splice(index, 1);
    }
    else {
        delete owner[key];
    }
}
/** Returns true if a value exists at the given pointer */
// prettier-ignore
function Has(value, pointer) {
    if (pointer === '')
        return true;
    let [owner, next, key] = [null, value, ''];
    for (const component of Format(pointer)) {
        if (next[component] === undefined)
            return false;
        owner = next;
        next = next[component];
        key = component;
    }
    return Object.getOwnPropertyNames(owner).includes(key);
}
/** Gets the value at the given pointer */
// prettier-ignore
function Get(value, pointer) {
    if (pointer === '')
        return value;
    let current = value;
    for (const component of Format(pointer)) {
        if (current[component] === undefined)
            return undefined;
        current = current[component];
    }
    return current;
}
