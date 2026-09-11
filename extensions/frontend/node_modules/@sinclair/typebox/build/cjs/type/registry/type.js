"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Entries = Entries;
exports.Clear = Clear;
exports.Delete = Delete;
exports.Has = Has;
exports.Set = Set;
exports.Get = Get;
/** A registry for user defined types */
const map = new Map();
/** Returns the entries in this registry */
function Entries() {
    return new Map(map);
}
/** Clears all user defined types */
function Clear() {
    return map.clear();
}
/** Deletes a registered type */
function Delete(kind) {
    return map.delete(kind);
}
/** Returns true if this registry contains this kind */
function Has(kind) {
    return map.has(kind);
}
/** Sets a validation function for a user defined type */
function Set(kind, func) {
    map.set(kind, func);
}
/** Gets a custom validation function for a user defined type */
function Get(kind) {
    return map.get(kind);
}
