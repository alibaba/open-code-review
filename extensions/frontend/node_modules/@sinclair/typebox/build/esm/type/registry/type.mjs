/** A registry for user defined types */
const map = new Map();
/** Returns the entries in this registry */
export function Entries() {
    return new Map(map);
}
/** Clears all user defined types */
export function Clear() {
    return map.clear();
}
/** Deletes a registered type */
export function Delete(kind) {
    return map.delete(kind);
}
/** Returns true if this registry contains this kind */
export function Has(kind) {
    return map.has(kind);
}
/** Sets a validation function for a user defined type */
export function Set(kind, func) {
    map.set(kind, func);
}
/** Gets a custom validation function for a user defined type */
export function Get(kind) {
    return map.get(kind);
}
