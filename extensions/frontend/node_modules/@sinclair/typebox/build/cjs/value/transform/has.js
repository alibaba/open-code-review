"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.HasTransform = HasTransform;
const index_1 = require("../deref/index");
const index_2 = require("../../type/symbols/index");
// ------------------------------------------------------------------
// KindGuard
// ------------------------------------------------------------------
const kind_1 = require("../../type/guard/kind");
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const index_3 = require("../guard/index");
// prettier-ignore
function FromArray(schema, references) {
    return (0, kind_1.IsTransform)(schema) || Visit(schema.items, references);
}
// prettier-ignore
function FromAsyncIterator(schema, references) {
    return (0, kind_1.IsTransform)(schema) || Visit(schema.items, references);
}
// prettier-ignore
function FromConstructor(schema, references) {
    return (0, kind_1.IsTransform)(schema) || Visit(schema.returns, references) || schema.parameters.some((schema) => Visit(schema, references));
}
// prettier-ignore
function FromFunction(schema, references) {
    return (0, kind_1.IsTransform)(schema) || Visit(schema.returns, references) || schema.parameters.some((schema) => Visit(schema, references));
}
// prettier-ignore
function FromIntersect(schema, references) {
    return (0, kind_1.IsTransform)(schema) || (0, kind_1.IsTransform)(schema.unevaluatedProperties) || schema.allOf.some((schema) => Visit(schema, references));
}
// prettier-ignore
function FromImport(schema, references) {
    const additional = globalThis.Object.getOwnPropertyNames(schema.$defs).reduce((result, key) => [...result, schema.$defs[key]], []);
    const target = schema.$defs[schema.$ref];
    return (0, kind_1.IsTransform)(schema) || Visit(target, [...additional, ...references]);
}
// prettier-ignore
function FromIterator(schema, references) {
    return (0, kind_1.IsTransform)(schema) || Visit(schema.items, references);
}
// prettier-ignore
function FromNot(schema, references) {
    return (0, kind_1.IsTransform)(schema) || Visit(schema.not, references);
}
// prettier-ignore
function FromObject(schema, references) {
    return ((0, kind_1.IsTransform)(schema) ||
        Object.values(schema.properties).some((schema) => Visit(schema, references)) ||
        ((0, kind_1.IsSchema)(schema.additionalProperties) && Visit(schema.additionalProperties, references)));
}
// prettier-ignore
function FromPromise(schema, references) {
    return (0, kind_1.IsTransform)(schema) || Visit(schema.item, references);
}
// prettier-ignore
function FromRecord(schema, references) {
    const pattern = Object.getOwnPropertyNames(schema.patternProperties)[0];
    const property = schema.patternProperties[pattern];
    return (0, kind_1.IsTransform)(schema) || Visit(property, references) || ((0, kind_1.IsSchema)(schema.additionalProperties) && (0, kind_1.IsTransform)(schema.additionalProperties));
}
// prettier-ignore
function FromRef(schema, references) {
    if ((0, kind_1.IsTransform)(schema))
        return true;
    return Visit((0, index_1.Deref)(schema, references), references);
}
// prettier-ignore
function FromThis(schema, references) {
    if ((0, kind_1.IsTransform)(schema))
        return true;
    return Visit((0, index_1.Deref)(schema, references), references);
}
// prettier-ignore
function FromTuple(schema, references) {
    return (0, kind_1.IsTransform)(schema) || (!(0, index_3.IsUndefined)(schema.items) && schema.items.some((schema) => Visit(schema, references)));
}
// prettier-ignore
function FromUnion(schema, references) {
    return (0, kind_1.IsTransform)(schema) || schema.anyOf.some((schema) => Visit(schema, references));
}
// prettier-ignore
function Visit(schema, references) {
    const references_ = (0, index_1.Pushref)(schema, references);
    const schema_ = schema;
    if (schema.$id && visited.has(schema.$id))
        return false;
    if (schema.$id)
        visited.add(schema.$id);
    switch (schema[index_2.Kind]) {
        case 'Array':
            return FromArray(schema_, references_);
        case 'AsyncIterator':
            return FromAsyncIterator(schema_, references_);
        case 'Constructor':
            return FromConstructor(schema_, references_);
        case 'Function':
            return FromFunction(schema_, references_);
        case 'Import':
            return FromImport(schema_, references_);
        case 'Intersect':
            return FromIntersect(schema_, references_);
        case 'Iterator':
            return FromIterator(schema_, references_);
        case 'Not':
            return FromNot(schema_, references_);
        case 'Object':
            return FromObject(schema_, references_);
        case 'Promise':
            return FromPromise(schema_, references_);
        case 'Record':
            return FromRecord(schema_, references_);
        case 'Ref':
            return FromRef(schema_, references_);
        case 'This':
            return FromThis(schema_, references_);
        case 'Tuple':
            return FromTuple(schema_, references_);
        case 'Union':
            return FromUnion(schema_, references_);
        default:
            return (0, kind_1.IsTransform)(schema);
    }
}
const visited = new Set();
/** Returns true if this schema contains a transform codec */
function HasTransform(schema, references) {
    visited.clear();
    return Visit(schema, references);
}
