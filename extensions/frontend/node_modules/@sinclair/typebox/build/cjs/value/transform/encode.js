"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TransformEncodeError = exports.TransformEncodeCheckError = void 0;
exports.TransformEncode = TransformEncode;
const policy_1 = require("../../system/policy");
const index_1 = require("../../type/symbols/index");
const index_2 = require("../../type/error/index");
const index_3 = require("../../type/keyof/index");
const index_4 = require("../deref/index");
const index_5 = require("../check/index");
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const index_6 = require("../guard/index");
// ------------------------------------------------------------------
// KindGuard
// ------------------------------------------------------------------
const kind_1 = require("../../type/guard/kind");
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
// prettier-ignore
class TransformEncodeCheckError extends index_2.TypeBoxError {
    constructor(schema, value, error) {
        super(`The encoded value does not match the expected schema`);
        this.schema = schema;
        this.value = value;
        this.error = error;
    }
}
exports.TransformEncodeCheckError = TransformEncodeCheckError;
// prettier-ignore
class TransformEncodeError extends index_2.TypeBoxError {
    constructor(schema, path, value, error) {
        super(`${error instanceof Error ? error.message : 'Unknown error'}`);
        this.schema = schema;
        this.path = path;
        this.value = value;
        this.error = error;
    }
}
exports.TransformEncodeError = TransformEncodeError;
// ------------------------------------------------------------------
// Encode
// ------------------------------------------------------------------
// prettier-ignore
function Default(schema, path, value) {
    try {
        return (0, kind_1.IsTransform)(schema) ? schema[index_1.TransformKind].Encode(value) : value;
    }
    catch (error) {
        throw new TransformEncodeError(schema, path, value, error);
    }
}
// prettier-ignore
function FromArray(schema, references, path, value) {
    const defaulted = Default(schema, path, value);
    return (0, index_6.IsArray)(defaulted)
        ? defaulted.map((value, index) => Visit(schema.items, references, `${path}/${index}`, value))
        : defaulted;
}
// prettier-ignore
function FromImport(schema, references, path, value) {
    const additional = globalThis.Object.values(schema.$defs);
    const target = schema.$defs[schema.$ref];
    const result = Default(schema, path, value);
    return Visit(target, [...references, ...additional], path, result);
}
// prettier-ignore
function FromIntersect(schema, references, path, value) {
    const defaulted = Default(schema, path, value);
    if (!(0, index_6.IsObject)(value) || (0, index_6.IsValueType)(value))
        return defaulted;
    const knownEntries = (0, index_3.KeyOfPropertyEntries)(schema);
    const knownKeys = knownEntries.map(entry => entry[0]);
    const knownProperties = { ...defaulted };
    for (const [knownKey, knownSchema] of knownEntries)
        if (knownKey in knownProperties) {
            knownProperties[knownKey] = Visit(knownSchema, references, `${path}/${knownKey}`, knownProperties[knownKey]);
        }
    if (!(0, kind_1.IsTransform)(schema.unevaluatedProperties)) {
        return knownProperties;
    }
    const unknownKeys = Object.getOwnPropertyNames(knownProperties);
    const unevaluatedProperties = schema.unevaluatedProperties;
    const properties = { ...knownProperties };
    for (const key of unknownKeys)
        if (!knownKeys.includes(key)) {
            properties[key] = Default(unevaluatedProperties, `${path}/${key}`, properties[key]);
        }
    return properties;
}
// prettier-ignore
function FromNot(schema, references, path, value) {
    return Default(schema.not, path, Default(schema, path, value));
}
// prettier-ignore
function FromObject(schema, references, path, value) {
    const defaulted = Default(schema, path, value);
    if (!(0, index_6.IsObject)(defaulted))
        return defaulted;
    const knownKeys = (0, index_3.KeyOfPropertyKeys)(schema);
    const knownProperties = { ...defaulted };
    for (const key of knownKeys) {
        if (!(0, index_6.HasPropertyKey)(knownProperties, key))
            continue;
        // if the property value is undefined, but the target is not, nor does it satisfy exact optional 
        // property policy, then we need to continue. This is a special case for optional property handling 
        // where a transforms wrapped in a optional modifiers should not run.
        if ((0, index_6.IsUndefined)(knownProperties[key]) && (!(0, kind_1.IsUndefined)(schema.properties[key]) ||
            policy_1.TypeSystemPolicy.IsExactOptionalProperty(knownProperties, key)))
            continue;
        // encode property
        knownProperties[key] = Visit(schema.properties[key], references, `${path}/${key}`, knownProperties[key]);
    }
    if (!(0, kind_1.IsSchema)(schema.additionalProperties)) {
        return knownProperties;
    }
    const unknownKeys = Object.getOwnPropertyNames(knownProperties);
    const additionalProperties = schema.additionalProperties;
    const properties = { ...knownProperties };
    for (const key of unknownKeys)
        if (!knownKeys.includes(key)) {
            properties[key] = Default(additionalProperties, `${path}/${key}`, properties[key]);
        }
    return properties;
}
// prettier-ignore
function FromRecord(schema, references, path, value) {
    const defaulted = Default(schema, path, value);
    if (!(0, index_6.IsObject)(value))
        return defaulted;
    const pattern = Object.getOwnPropertyNames(schema.patternProperties)[0];
    const knownKeys = new RegExp(pattern);
    const knownProperties = { ...defaulted };
    for (const key of Object.getOwnPropertyNames(value))
        if (knownKeys.test(key)) {
            knownProperties[key] = Visit(schema.patternProperties[pattern], references, `${path}/${key}`, knownProperties[key]);
        }
    if (!(0, kind_1.IsSchema)(schema.additionalProperties)) {
        return knownProperties;
    }
    const unknownKeys = Object.getOwnPropertyNames(knownProperties);
    const additionalProperties = schema.additionalProperties;
    const properties = { ...knownProperties };
    for (const key of unknownKeys)
        if (!knownKeys.test(key)) {
            properties[key] = Default(additionalProperties, `${path}/${key}`, properties[key]);
        }
    return properties;
}
// prettier-ignore
function FromRef(schema, references, path, value) {
    const target = (0, index_4.Deref)(schema, references);
    const resolved = Visit(target, references, path, value);
    return Default(schema, path, resolved);
}
// prettier-ignore
function FromThis(schema, references, path, value) {
    const target = (0, index_4.Deref)(schema, references);
    const resolved = Visit(target, references, path, value);
    return Default(schema, path, resolved);
}
// prettier-ignore
function FromTuple(schema, references, path, value) {
    const value1 = Default(schema, path, value);
    return (0, index_6.IsArray)(schema.items) ? schema.items.map((schema, index) => Visit(schema, references, `${path}/${index}`, value1[index])) : [];
}
// prettier-ignore
function FromUnion(schema, references, path, value) {
    // test value against union variants
    for (const subschema of schema.anyOf) {
        if (!(0, index_5.Check)(subschema, references, value))
            continue;
        const value1 = Visit(subschema, references, path, value);
        return Default(schema, path, value1);
    }
    // test transformed value against union variants
    for (const subschema of schema.anyOf) {
        const value1 = Visit(subschema, references, path, value);
        if (!(0, index_5.Check)(schema, references, value1))
            continue;
        return Default(schema, path, value1);
    }
    return Default(schema, path, value);
}
// prettier-ignore
function Visit(schema, references, path, value) {
    const references_ = (0, index_4.Pushref)(schema, references);
    const schema_ = schema;
    switch (schema[index_1.Kind]) {
        case 'Array':
            return FromArray(schema_, references_, path, value);
        case 'Import':
            return FromImport(schema_, references_, path, value);
        case 'Intersect':
            return FromIntersect(schema_, references_, path, value);
        case 'Not':
            return FromNot(schema_, references_, path, value);
        case 'Object':
            return FromObject(schema_, references_, path, value);
        case 'Record':
            return FromRecord(schema_, references_, path, value);
        case 'Ref':
            return FromRef(schema_, references_, path, value);
        case 'This':
            return FromThis(schema_, references_, path, value);
        case 'Tuple':
            return FromTuple(schema_, references_, path, value);
        case 'Union':
            return FromUnion(schema_, references_, path, value);
        default:
            return Default(schema_, path, value);
    }
}
/**
 * `[Internal]` Encodes the value and returns the result. This function expects the
 * caller to pass a statically checked value. This function does not check the encoded
 * result, meaning the result should be passed to `Check` before use. Refer to the
 * `Value.Encode()` function for implementation details.
 */
function TransformEncode(schema, references, value) {
    return Visit(schema, references, '', value);
}
