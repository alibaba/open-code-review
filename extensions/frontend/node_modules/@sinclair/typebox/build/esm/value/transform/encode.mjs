import { TypeSystemPolicy } from '../../system/policy.mjs';
import { Kind, TransformKind } from '../../type/symbols/index.mjs';
import { TypeBoxError } from '../../type/error/index.mjs';
import { KeyOfPropertyKeys, KeyOfPropertyEntries } from '../../type/keyof/index.mjs';
import { Deref, Pushref } from '../deref/index.mjs';
import { Check } from '../check/index.mjs';
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
import { HasPropertyKey, IsObject, IsArray, IsValueType, IsUndefined as IsUndefinedValue } from '../guard/index.mjs';
// ------------------------------------------------------------------
// KindGuard
// ------------------------------------------------------------------
import { IsTransform, IsSchema, IsUndefined } from '../../type/guard/kind.mjs';
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
// prettier-ignore
export class TransformEncodeCheckError extends TypeBoxError {
    constructor(schema, value, error) {
        super(`The encoded value does not match the expected schema`);
        this.schema = schema;
        this.value = value;
        this.error = error;
    }
}
// prettier-ignore
export class TransformEncodeError extends TypeBoxError {
    constructor(schema, path, value, error) {
        super(`${error instanceof Error ? error.message : 'Unknown error'}`);
        this.schema = schema;
        this.path = path;
        this.value = value;
        this.error = error;
    }
}
// ------------------------------------------------------------------
// Encode
// ------------------------------------------------------------------
// prettier-ignore
function Default(schema, path, value) {
    try {
        return IsTransform(schema) ? schema[TransformKind].Encode(value) : value;
    }
    catch (error) {
        throw new TransformEncodeError(schema, path, value, error);
    }
}
// prettier-ignore
function FromArray(schema, references, path, value) {
    const defaulted = Default(schema, path, value);
    return IsArray(defaulted)
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
    if (!IsObject(value) || IsValueType(value))
        return defaulted;
    const knownEntries = KeyOfPropertyEntries(schema);
    const knownKeys = knownEntries.map(entry => entry[0]);
    const knownProperties = { ...defaulted };
    for (const [knownKey, knownSchema] of knownEntries)
        if (knownKey in knownProperties) {
            knownProperties[knownKey] = Visit(knownSchema, references, `${path}/${knownKey}`, knownProperties[knownKey]);
        }
    if (!IsTransform(schema.unevaluatedProperties)) {
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
    if (!IsObject(defaulted))
        return defaulted;
    const knownKeys = KeyOfPropertyKeys(schema);
    const knownProperties = { ...defaulted };
    for (const key of knownKeys) {
        if (!HasPropertyKey(knownProperties, key))
            continue;
        // if the property value is undefined, but the target is not, nor does it satisfy exact optional 
        // property policy, then we need to continue. This is a special case for optional property handling 
        // where a transforms wrapped in a optional modifiers should not run.
        if (IsUndefinedValue(knownProperties[key]) && (!IsUndefined(schema.properties[key]) ||
            TypeSystemPolicy.IsExactOptionalProperty(knownProperties, key)))
            continue;
        // encode property
        knownProperties[key] = Visit(schema.properties[key], references, `${path}/${key}`, knownProperties[key]);
    }
    if (!IsSchema(schema.additionalProperties)) {
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
    if (!IsObject(value))
        return defaulted;
    const pattern = Object.getOwnPropertyNames(schema.patternProperties)[0];
    const knownKeys = new RegExp(pattern);
    const knownProperties = { ...defaulted };
    for (const key of Object.getOwnPropertyNames(value))
        if (knownKeys.test(key)) {
            knownProperties[key] = Visit(schema.patternProperties[pattern], references, `${path}/${key}`, knownProperties[key]);
        }
    if (!IsSchema(schema.additionalProperties)) {
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
    const target = Deref(schema, references);
    const resolved = Visit(target, references, path, value);
    return Default(schema, path, resolved);
}
// prettier-ignore
function FromThis(schema, references, path, value) {
    const target = Deref(schema, references);
    const resolved = Visit(target, references, path, value);
    return Default(schema, path, resolved);
}
// prettier-ignore
function FromTuple(schema, references, path, value) {
    const value1 = Default(schema, path, value);
    return IsArray(schema.items) ? schema.items.map((schema, index) => Visit(schema, references, `${path}/${index}`, value1[index])) : [];
}
// prettier-ignore
function FromUnion(schema, references, path, value) {
    // test value against union variants
    for (const subschema of schema.anyOf) {
        if (!Check(subschema, references, value))
            continue;
        const value1 = Visit(subschema, references, path, value);
        return Default(schema, path, value1);
    }
    // test transformed value against union variants
    for (const subschema of schema.anyOf) {
        const value1 = Visit(subschema, references, path, value);
        if (!Check(schema, references, value1))
            continue;
        return Default(schema, path, value1);
    }
    return Default(schema, path, value);
}
// prettier-ignore
function Visit(schema, references, path, value) {
    const references_ = Pushref(schema, references);
    const schema_ = schema;
    switch (schema[Kind]) {
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
export function TransformEncode(schema, references, value) {
    return Visit(schema, references, '', value);
}
