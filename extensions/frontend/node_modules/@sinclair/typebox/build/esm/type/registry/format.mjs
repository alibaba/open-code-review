/** A registry for user defined string formats */
const map = new Map();
/** Returns the entries in this registry */
export function Entries() {
    return new Map(map);
}
/** Clears all user defined string formats */
export function Clear() {
    return map.clear();
}
/** Deletes a registered format */
export function Delete(format) {
    return map.delete(format);
}
/** Returns true if the user defined string format exists */
export function Has(format) {
    return map.has(format);
}
/** Sets a validation function for a user defined string format */
export function Set(format, func) {
    map.set(format, func);
}
/** Gets a validation function for a user defined string format */
export function Get(format) {
    return map.get(format);
}
