"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.KeyOfPropertyKeys = KeyOfPropertyKeys;
exports.KeyOfPattern = KeyOfPattern;
const index_1 = require("../sets/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function FromRest(types) {
    const result = [];
    for (const L of types)
        result.push(KeyOfPropertyKeys(L));
    return result;
}
// prettier-ignore
function FromIntersect(types) {
    const propertyKeysArray = FromRest(types);
    const propertyKeys = (0, index_1.SetUnionMany)(propertyKeysArray);
    return propertyKeys;
}
// prettier-ignore
function FromUnion(types) {
    const propertyKeysArray = FromRest(types);
    const propertyKeys = (0, index_1.SetIntersectMany)(propertyKeysArray);
    return propertyKeys;
}
// prettier-ignore
function FromTuple(types) {
    return types.map((_, indexer) => indexer.toString());
}
// prettier-ignore
function FromArray(_) {
    return (['[number]']);
}
// prettier-ignore
function FromProperties(T) {
    return (globalThis.Object.getOwnPropertyNames(T));
}
// ------------------------------------------------------------------
// FromPatternProperties
// ------------------------------------------------------------------
// prettier-ignore
function FromPatternProperties(patternProperties) {
    if (!includePatternProperties)
        return [];
    const patternPropertyKeys = globalThis.Object.getOwnPropertyNames(patternProperties);
    return patternPropertyKeys.map(key => {
        return (key[0] === '^' && key[key.length - 1] === '$')
            ? key.slice(1, key.length - 1)
            : key;
    });
}
/** Returns a tuple of PropertyKeys derived from the given TSchema. */
// prettier-ignore
function KeyOfPropertyKeys(type) {
    return ((0, kind_1.IsIntersect)(type) ? FromIntersect(type.allOf) :
        (0, kind_1.IsUnion)(type) ? FromUnion(type.anyOf) :
            (0, kind_1.IsTuple)(type) ? FromTuple(type.items ?? []) :
                (0, kind_1.IsArray)(type) ? FromArray(type.items) :
                    (0, kind_1.IsObject)(type) ? FromProperties(type.properties) :
                        (0, kind_1.IsRecord)(type) ? FromPatternProperties(type.patternProperties) :
                            []);
}
// ----------------------------------------------------------------
// KeyOfPattern
// ----------------------------------------------------------------
let includePatternProperties = false;
/** Returns a regular expression pattern derived from the given TSchema */
function KeyOfPattern(schema) {
    includePatternProperties = true;
    const keys = KeyOfPropertyKeys(schema);
    includePatternProperties = false;
    const pattern = keys.map((key) => `(${key})`);
    return `^(${pattern.join('|')})$`;
}
