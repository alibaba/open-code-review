"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Entries = Entries;
exports.Clear = Clear;
exports.Delete = Delete;
exports.Has = Has;
exports.Set = Set;
exports.Get = Get;
/** A registry for user defined string formats */
const map = new Map();
/** Returns the entries in this registry */
function Entries() {
    return new Map(map);
}
/** Clears all user defined string formats */
function Clear() {
    return map.clear();
}
/** Deletes a registered format */
function Delete(format) {
    return map.delete(format);
}
/** Returns true if the user defined string format exists */
function Has(format) {
    return map.has(format);
}
/** Sets a validation function for a user defined string format */
function Set(format, func) {
    map.set(format, func);
}
/** Gets a validation function for a user defined string format */
function Get(format) {
    return map.get(format);
}
