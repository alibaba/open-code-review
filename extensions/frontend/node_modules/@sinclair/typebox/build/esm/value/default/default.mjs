import { Check } from '../check/index.mjs';
import { Clone } from '../clone/index.mjs';
import { Deref, Pushref } from '../deref/index.mjs';
import { Kind } from '../../type/symbols/index.mjs';
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
import { IsArray, IsDate, IsFunction, IsObject, IsUndefined, HasPropertyKey } from '../guard/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsKind } from '../../type/guard/kind.mjs';
// ------------------------------------------------------------------
// ValueOrDefault
// ------------------------------------------------------------------
function ValueOrDefault(schema, value) {
    const defaultValue = HasPropertyKey(schema, 'default') ? schema.default : undefined;
    const clone = IsFunction(defaultValue) ? defaultValue() : Clone(defaultValue);
    return IsUndefined(value) ? clone : IsObject(value) && IsObject(clone) ? Object.assign(clone, value) : value;
}
// ------------------------------------------------------------------
// HasDefaultProperty
// ------------------------------------------------------------------
function HasDefaultProperty(schema) {
    return IsKind(schema) && 'default' in schema;
}
// ------------------------------------------------------------------
// Types
// ------------------------------------------------------------------
function FromArray(schema, references, value) {
    // if the value is an array, we attempt to initialize it's elements
    if (IsArray(value)) {
        for (let i = 0; i < value.length; i++) {
            value[i] = Visit(schema.items, references, value[i]);
        }
        return value;
    }
    // ... otherwise use default initialization
    const defaulted = ValueOrDefault(schema, value);
    if (!IsArray(defaulted))
        return defaulted;
    for (let i = 0; i < defaulted.length; i++) {
        defaulted[i] = Visit(schema.items, references, defaulted[i]);
    }
    return defaulted;
}
function FromDate(schema, references, value) {
    // special case intercept for dates
    return IsDate(value) ? value : ValueOrDefault(schema, value);
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
        return IsObject(next) ? { ...acc, ...next } : next;
    }, {});
}
function FromObject(schema, references, value) {
    const defaulted = ValueOrDefault(schema, value);
    // return defaulted
    if (!IsObject(defaulted))
        return defaulted;
    const knownPropertyKeys = Object.getOwnPropertyNames(schema.properties);
    // properties
    for (const key of knownPropertyKeys) {
        // note: we need to traverse into the object and test if the return value
        // yielded a non undefined result. Here we interpret an undefined result as
        // a non assignable property and continue.
        const propertyValue = Visit(schema.properties[key], references, defaulted[key]);
        if (IsUndefined(propertyValue))
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
    if (!IsObject(defaulted))
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
    return Visit(Deref(schema, references), references, ValueOrDefault(schema, value));
}
function FromThis(schema, references, value) {
    return Visit(Deref(schema, references), references, value);
}
function FromTuple(schema, references, value) {
    const defaulted = ValueOrDefault(schema, value);
    if (!IsArray(defaulted) || IsUndefined(schema.items))
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
        const result = Visit(inner, references, Clone(defaulted));
        if (Check(inner, references, result)) {
            return result;
        }
    }
    return defaulted;
}
function Visit(schema, references, value) {
    const references_ = Pushref(schema, references);
    const schema_ = schema;
    switch (schema_[Kind]) {
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
export function Default(...args) {
    return args.length === 3 ? Visit(args[0], args[1], args[2]) : Visit(args[0], [], args[1]);
}
