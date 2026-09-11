"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Default = Default;
const index_1 = require("../check/index");
const index_2 = require("../clone/index");
const index_3 = require("../deref/index");
const index_4 = require("../../type/symbols/index");
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const index_5 = require("../guard/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../../type/guard/kind");
// ------------------------------------------------------------------
// ValueOrDefault
// ------------------------------------------------------------------
function ValueOrDefault(schema, value) {
    const defaultValue = (0, index_5.HasPropertyKey)(schema, 'default') ? schema.default : undefined;
    const clone = (0, index_5.IsFunction)(defaultValue) ? defaultValue() : (0, index_2.Clone)(defaultValue);
    return (0, index_5.IsUndefined)(value) ? clone : (0, index_5.IsObject)(value) && (0, index_5.IsObject)(clone) ? Object.assign(clone, value) : value;
}
// ------------------------------------------------------------------
// HasDefaultProperty
// ------------------------------------------------------------------
function HasDefaultProperty(schema) {
    return (0, kind_1.IsKind)(schema) && 'default' in schema;
}
// ------------------------------------------------------------------
// Types
// ------------------------------------------------------------------
function FromArray(schema, references, value) {
    // if the value is an array, we attempt to initialize it's elements
    if ((0, index_5.IsArray)(value)) {
        for (let i = 0; i < value.length; i++) {
            value[i] = Visit(schema.items, references, value[i]);
        }
        return value;
    }
    // ... otherwise use default initialization
    const defaulted = ValueOrDefault(schema, value);
    if (!(0, index_5.IsArray)(defaulted))
        return defaulted;
    for (let i = 0; i < defaulted.length; i++) {
        defaulted[i] = Visit(schema.items, references, defaulted[i]);
    }
    return defaulted;
}
function FromDate(schema, references, value) {
    // special case intercept for dates
    return (0, index_5.IsDate)(value) ? value : ValueOrDefault(schema, value);
}
function FromImport(schema, references, value) {
    const definitions = globalThis.Object.values(schema.$defs);
    const target = schema.$defs[schema.$ref];
    return Visit(target, [...references, ...definitions], value);
}
function FromIntersect(schema, references, value) {
    const defaulted = ValueOrDefault(schema, value);
    return schema.allOf.reduce((acc, schema) => {
        const next = Visit(schema, references, defaulted);
        return (0, index_5.IsObject)(next) ? { ...acc, ...next } : next;
    }, {});
}
function FromObject(schema, references, value) {
    const defaulted = ValueOrDefault(schema, value);
    // return defaulted
    if (!(0, index_5.IsObject)(defaulted))
        return defaulted;
    const knownPropertyKeys = Object.getOwnPropertyNames(schema.properties);
    // properties
    for (const key of knownPropertyKeys) {
        // note: we need to traverse into the object and test if the return value
        // yielded a non undefined result. Here we interpret an undefined result as
        // a non assignable property and continue.
        const propertyValue = Visit(schema.properties[key], references, defaulted[key]);
        if ((0, index_5.IsUndefined)(propertyValue))
            continue;
        defaulted[key] = Visit(schema.properties[key], references, defaulted[key]);
    }
    // return if not additional properties
    if (!HasDefaultProperty(schema.additionalProperties))
        return defaulted;
    // additional properties
    for (const key of Object.getOwnPropertyNames(defaulted)) {
        if (knownPropertyKeys.includes(key))
            continue;
        defaulted[key] = Visit(schema.additionalProperties, references, defaulted[key]);
    }
    return defaulted;
}
function FromRecord(schema, references, value) {
    const defaulted = ValueOrDefault(schema, value);
    if (!(0, index_5.IsObject)(defaulted))
        return defaulted;
    const additionalPropertiesSchema = schema.additionalProperties;
    const [propertyKeyPattern, propertySchema] = Object.entries(schema.patternProperties)[0];
    const knownPropertyKey = new RegExp(propertyKeyPattern);
    // properties
    for (const key of Object.getOwnPropertyNames(defaulted)) {
        if (!(knownPropertyKey.test(key) && HasDefaultProperty(propertySchema)))
            continue;
        defaulted[key] = Visit(propertySchema, references, defaulted[key]);
    }
    // return if not additional properties
    if (!HasDefaultProperty(additionalPropertiesSchema))
        return defaulted;
    // additional properties
    for (const key of Object.getOwnPropertyNames(defaulted)) {
        if (knownPropertyKey.test(key))
            continue;
        defaulted[key] = Visit(additionalPropertiesSchema, references, defaulted[key]);
    }
    return defaulted;
}
function FromRef(schema, references, value) {
    return Visit((0, index_3.Deref)(schema, references), references, ValueOrDefault(schema, value));
}
function FromThis(schema, references, value) {
    return Visit((0, index_3.Deref)(schema, references), references, value);
}
function FromTuple(schema, references, value) {
    const defaulted = ValueOrDefault(schema, value);
    if (!(0, index_5.IsArray)(defaulted) || (0, index_5.IsUndefined)(schema.items))
        return defaulted;
    const [items, max] = [schema.items, Math.max(schema.items.length, defaulted.length)];
    for (let i = 0; i < max; i++) {
        if (i < items.length)
            defaulted[i] = Visit(items[i], references, defaulted[i]);
    }
    return defaulted;
}
function FromUnion(schema, references, value) {
    const defaulted = ValueOrDefault(schema, value);
    for (const inner of schema.anyOf) {
        const result = Visit(inner, references, (0, index_2.Clone)(defaulted));
        if ((0, index_1.Check)(inner, references, result)) {
            return result;
        }
    }
    return defaulted;
}
function Visit(schema, references, value) {
    const references_ = (0, index_3.Pushref)(schema, references);
    const schema_ = schema;
    switch (schema_[index_4.Kind]) {
        case 'Array':
            return FromArray(schema_, references_, value);
        case 'Date':
            return FromDate(schema_, references_, value);
        case 'Import':
            return FromImport(schema_, references_, value);
        case 'Intersect':
            return FromIntersect(schema_, references_, value);
        case 'Object':
            return FromObject(schema_, references_, value);
        case 'Record':
            return FromRecord(schema_, references_, value);
        case 'Ref':
            return FromRef(schema_, references_, value);
        case 'This':
            return FromThis(schema_, references_, value);
        case 'Tuple':
            return FromTuple(schema_, references_, value);
        case 'Union':
            return FromUnion(schema_, references_, value);
        default:
            return ValueOrDefault(schema_, value);
    }
}
/** `[Mutable]` Generates missing properties on a value using default schema annotations if available. This function does not check the value and returns an unknown type. You should Check the result before use. Default is a mutable operation. To avoid mutation, Clone the value first. */
function Default(...args) {
    return args.length === 3 ? Visit(args[0], args[1], args[2]) : Visit(args[0], [], args[1]);
}
