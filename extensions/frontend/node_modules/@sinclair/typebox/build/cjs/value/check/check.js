"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ValueCheckUnknownTypeError = void 0;
exports.Check = Check;
const index_1 = require("../../system/index");
const index_2 = require("../deref/index");
const index_3 = require("../hash/index");
const index_4 = require("../../type/symbols/index");
const index_5 = require("../../type/keyof/index");
const index_6 = require("../../type/extends/index");
const index_7 = require("../../type/registry/index");
const index_8 = require("../../type/error/index");
const index_9 = require("../../type/never/index");
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
const index_10 = require("../guard/index");
// ------------------------------------------------------------------
// KindGuard
// ------------------------------------------------------------------
const kind_1 = require("../../type/guard/kind");
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
class ValueCheckUnknownTypeError extends index_8.TypeBoxError {
    constructor(schema) {
        super(`Unknown type`);
        this.schema = schema;
    }
}
exports.ValueCheckUnknownTypeError = ValueCheckUnknownTypeError;
// ------------------------------------------------------------------
// TypeGuards
// ------------------------------------------------------------------
function IsAnyOrUnknown(schema) {
    return schema[index_4.Kind] === 'Any' || schema[index_4.Kind] === 'Unknown';
}
// ------------------------------------------------------------------
// Guards
// ------------------------------------------------------------------
function IsDefined(value) {
    return value !== undefined;
}
// ------------------------------------------------------------------
// Types
// ------------------------------------------------------------------
function FromAny(schema, references, value) {
    return true;
}
function FromArgument(schema, references, value) {
    return true;
}
function FromArray(schema, references, value) {
    if (!(0, index_10.IsArray)(value))
        return false;
    if (IsDefined(schema.minItems) && !(value.length >= schema.minItems)) {
        return false;
    }
    if (IsDefined(schema.maxItems) && !(value.length <= schema.maxItems)) {
        return false;
    }
    for (const element of value) {
        if (!Visit(schema.items, references, element))
            return false;
    }
    // prettier-ignore
    if (schema.uniqueItems === true && !((function () { const set = new Set(); for (const element of value) {
        const hashed = (0, index_3.Hash)(element);
        if (set.has(hashed)) {
            return false;
        }
        else {
            set.add(hashed);
        }
    } return true; })())) {
        return false;
    }
    // contains
    if (!(IsDefined(schema.contains) || (0, index_10.IsNumber)(schema.minContains) || (0, index_10.IsNumber)(schema.maxContains))) {
        return true; // exit
    }
    const containsSchema = IsDefined(schema.contains) ? schema.contains : (0, index_9.Never)();
    const containsCount = value.reduce((acc, value) => (Visit(containsSchema, references, value) ? acc + 1 : acc), 0);
    if (containsCount === 0) {
        return false;
    }
    if ((0, index_10.IsNumber)(schema.minContains) && containsCount < schema.minContains) {
        return false;
    }
    if ((0, index_10.IsNumber)(schema.maxContains) && containsCount > schema.maxContains) {
        return false;
    }
    return true;
}
function FromAsyncIterator(schema, references, value) {
    return (0, index_10.IsAsyncIterator)(value);
}
function FromBigInt(schema, references, value) {
    if (!(0, index_10.IsBigInt)(value))
        return false;
    if (IsDefined(schema.exclusiveMaximum) && !(value < schema.exclusiveMaximum)) {
        return false;
    }
    if (IsDefined(schema.exclusiveMinimum) && !(value > schema.exclusiveMinimum)) {
        return false;
    }
    if (IsDefined(schema.maximum) && !(value <= schema.maximum)) {
        return false;
    }
    if (IsDefined(schema.minimum) && !(value >= schema.minimum)) {
        return false;
    }
    if (IsDefined(schema.multipleOf) && !(value % schema.multipleOf === BigInt(0))) {
        return false;
    }
    return true;
}
function FromBoolean(schema, references, value) {
    return (0, index_10.IsBoolean)(value);
}
function FromConstructor(schema, references, value) {
    return Visit(schema.returns, references, value.prototype);
}
function FromDate(schema, references, value) {
    if (!(0, index_10.IsDate)(value))
        return false;
    if (IsDefined(schema.exclusiveMaximumTimestamp) && !(value.getTime() < schema.exclusiveMaximumTimestamp)) {
        return false;
    }
    if (IsDefined(schema.exclusiveMinimumTimestamp) && !(value.getTime() > schema.exclusiveMinimumTimestamp)) {
        return false;
    }
    if (IsDefined(schema.maximumTimestamp) && !(value.getTime() <= schema.maximumTimestamp)) {
        return false;
    }
    if (IsDefined(schema.minimumTimestamp) && !(value.getTime() >= schema.minimumTimestamp)) {
        return false;
    }
    if (IsDefined(schema.multipleOfTimestamp) && !(value.getTime() % schema.multipleOfTimestamp === 0)) {
        return false;
    }
    return true;
}
function FromFunction(schema, references, value) {
    return (0, index_10.IsFunction)(value);
}
function FromImport(schema, references, value) {
    const definitions = globalThis.Object.values(schema.$defs);
    const target = schema.$defs[schema.$ref];
    return Visit(target, [...references, ...definitions], value);
}
function FromInteger(schema, references, value) {
    if (!(0, index_10.IsInteger)(value)) {
        return false;
    }
    if (IsDefined(schema.exclusiveMaximum) && !(value < schema.exclusiveMaximum)) {
        return false;
    }
    if (IsDefined(schema.exclusiveMinimum) && !(value > schema.exclusiveMinimum)) {
        return false;
    }
    if (IsDefined(schema.maximum) && !(value <= schema.maximum)) {
        return false;
    }
    if (IsDefined(schema.minimum) && !(value >= schema.minimum)) {
        return false;
    }
    if (IsDefined(schema.multipleOf) && !(value % schema.multipleOf === 0)) {
        return false;
    }
    return true;
}
function FromIntersect(schema, references, value) {
    const check1 = schema.allOf.every((schema) => Visit(schema, references, value));
    if (schema.unevaluatedProperties === false) {
        const keyPattern = new RegExp((0, index_5.KeyOfPattern)(schema));
        const check2 = Object.getOwnPropertyNames(value).every((key) => keyPattern.test(key));
        return check1 && check2;
    }
    else if ((0, kind_1.IsSchema)(schema.unevaluatedProperties)) {
        const keyCheck = new RegExp((0, index_5.KeyOfPattern)(schema));
        const check2 = Object.getOwnPropertyNames(value).every((key) => keyCheck.test(key) || Visit(schema.unevaluatedProperties, references, value[key]));
        return check1 && check2;
    }
    else {
        return check1;
    }
}
function FromIterator(schema, references, value) {
    return (0, index_10.IsIterator)(value);
}
function FromLiteral(schema, references, value) {
    return value === schema.const;
}
function FromNever(schema, references, value) {
    return false;
}
function FromNot(schema, references, value) {
    return !Visit(schema.not, references, value);
}
function FromNull(schema, references, value) {
    return (0, index_10.IsNull)(value);
}
function FromNumber(schema, references, value) {
    if (!index_1.TypeSystemPolicy.IsNumberLike(value))
        return false;
    if (IsDefined(schema.exclusiveMaximum) && !(value < schema.exclusiveMaximum)) {
        return false;
    }
    if (IsDefined(schema.exclusiveMinimum) && !(value > schema.exclusiveMinimum)) {
        return false;
    }
    if (IsDefined(schema.minimum) && !(value >= schema.minimum)) {
        return false;
    }
    if (IsDefined(schema.maximum) && !(value <= schema.maximum)) {
        return false;
    }
    if (IsDefined(schema.multipleOf) && !(value % schema.multipleOf === 0)) {
        return false;
    }
    return true;
}
function FromObject(schema, references, value) {
    if (!index_1.TypeSystemPolicy.IsObjectLike(value))
        return false;
    if (IsDefined(schema.minProperties) && !(Object.getOwnPropertyNames(value).length >= schema.minProperties)) {
        return false;
    }
    if (IsDefined(schema.maxProperties) && !(Object.getOwnPropertyNames(value).length <= schema.maxProperties)) {
        return false;
    }
    const knownKeys = Object.getOwnPropertyNames(schema.properties);
    for (const knownKey of knownKeys) {
        const property = schema.properties[knownKey];
        if (schema.required && schema.required.includes(knownKey)) {
            if (!Visit(property, references, value[knownKey])) {
                return false;
            }
            if (((0, index_6.ExtendsUndefinedCheck)(property) || IsAnyOrUnknown(property)) && !(knownKey in value)) {
                return false;
            }
        }
        else {
            if (index_1.TypeSystemPolicy.IsExactOptionalProperty(value, knownKey) && !Visit(property, references, value[knownKey])) {
                return false;
            }
        }
    }
    if (schema.additionalProperties === false) {
        const valueKeys = Object.getOwnPropertyNames(value);
        // optimization: value is valid if schemaKey length matches the valueKey length
        if (schema.required && schema.required.length === knownKeys.length && valueKeys.length === knownKeys.length) {
            return true;
        }
        else {
            return valueKeys.every((valueKey) => knownKeys.includes(valueKey));
        }
    }
    else if (typeof schema.additionalProperties === 'object') {
        const valueKeys = Object.getOwnPropertyNames(value);
        return valueKeys.every((key) => knownKeys.includes(key) || Visit(schema.additionalProperties, references, value[key]));
    }
    else {
        return true;
    }
}
function FromPromise(schema, references, value) {
    return (0, index_10.IsPromise)(value);
}
function FromRecord(schema, references, value) {
    if (!index_1.TypeSystemPolicy.IsRecordLike(value)) {
        return false;
    }
    if (IsDefined(schema.minProperties) && !(Object.getOwnPropertyNames(value).length >= schema.minProperties)) {
        return false;
    }
    if (IsDefined(schema.maxProperties) && !(Object.getOwnPropertyNames(value).length <= schema.maxProperties)) {
        return false;
    }
    const [patternKey, patternSchema] = Object.entries(schema.patternProperties)[0];
    const regex = new RegExp(patternKey);
    // prettier-ignore
    const check1 = Object.entries(value).every(([key, value]) => {
        return (regex.test(key)) ? Visit(patternSchema, references, value) : true;
    });
    // prettier-ignore
    const check2 = typeof schema.additionalProperties === 'object' ? Object.entries(value).every(([key, value]) => {
        return (!regex.test(key)) ? Visit(schema.additionalProperties, references, value) : true;
    }) : true;
    const check3 = schema.additionalProperties === false
        ? Object.getOwnPropertyNames(value).every((key) => {
            return regex.test(key);
        })
        : true;
    return check1 && check2 && check3;
}
function FromRef(schema, references, value) {
    return Visit((0, index_2.Deref)(schema, references), references, value);
}
function FromRegExp(schema, references, value) {
    const regex = new RegExp(schema.source, schema.flags);
    if (IsDefined(schema.minLength)) {
        if (!(value.length >= schema.minLength))
            return false;
    }
    if (IsDefined(schema.maxLength)) {
        if (!(value.length <= schema.maxLength))
            return false;
    }
    return regex.test(value);
}
function FromString(schema, references, value) {
    if (!(0, index_10.IsString)(value)) {
        return false;
    }
    if (IsDefined(schema.minLength)) {
        if (!(value.length >= schema.minLength))
            return false;
    }
    if (IsDefined(schema.maxLength)) {
        if (!(value.length <= schema.maxLength))
            return false;
    }
    if (IsDefined(schema.pattern)) {
        const regex = new RegExp(schema.pattern);
        if (!regex.test(value))
            return false;
    }
    if (IsDefined(schema.format)) {
        if (!index_7.FormatRegistry.Has(schema.format))
            return false;
        const func = index_7.FormatRegistry.Get(schema.format);
        return func(value);
    }
    return true;
}
function FromSymbol(schema, references, value) {
    return (0, index_10.IsSymbol)(value);
}
function FromTemplateLiteral(schema, references, value) {
    return (0, index_10.IsString)(value) && new RegExp(schema.pattern).test(value);
}
function FromThis(schema, references, value) {
    return Visit((0, index_2.Deref)(schema, references), references, value);
}
function FromTuple(schema, references, value) {
    if (!(0, index_10.IsArray)(value)) {
        return false;
    }
    if (schema.items === undefined && !(value.length === 0)) {
        return false;
    }
    if (!(value.length === schema.maxItems)) {
        return false;
    }
    if (!schema.items) {
        return true;
    }
    for (let i = 0; i < schema.items.length; i++) {
        if (!Visit(schema.items[i], references, value[i]))
            return false;
    }
    return true;
}
function FromUndefined(schema, references, value) {
    return (0, index_10.IsUndefined)(value);
}
function FromUnion(schema, references, value) {
    return schema.anyOf.some((inner) => Visit(inner, references, value));
}
function FromUint8Array(schema, references, value) {
    if (!(0, index_10.IsUint8Array)(value)) {
        return false;
    }
    if (IsDefined(schema.maxByteLength) && !(value.length <= schema.maxByteLength)) {
        return false;
    }
    if (IsDefined(schema.minByteLength) && !(value.length >= schema.minByteLength)) {
        return false;
    }
    return true;
}
function FromUnknown(schema, references, value) {
    return true;
}
function FromVoid(schema, references, value) {
    return index_1.TypeSystemPolicy.IsVoidLike(value);
}
function FromKind(schema, references, value) {
    if (!index_7.TypeRegistry.Has(schema[index_4.Kind]))
        return false;
    const func = index_7.TypeRegistry.Get(schema[index_4.Kind]);
    return func(schema, value);
}
function Visit(schema, references, value) {
    const references_ = IsDefined(schema.$id) ? (0, index_2.Pushref)(schema, references) : references;
    const schema_ = schema;
    switch (schema_[index_4.Kind]) {
        case 'Any':
            return FromAny(schema_, references_, value);
        case 'Argument':
            return FromArgument(schema_, references_, value);
        case 'Array':
            return FromArray(schema_, references_, value);
        case 'AsyncIterator':
            return FromAsyncIterator(schema_, references_, value);
        case 'BigInt':
            return FromBigInt(schema_, references_, value);
        case 'Boolean':
            return FromBoolean(schema_, references_, value);
        case 'Constructor':
            return FromConstructor(schema_, references_, value);
        case 'Date':
            return FromDate(schema_, references_, value);
        case 'Function':
            return FromFunction(schema_, references_, value);
        case 'Import':
            return FromImport(schema_, references_, value);
        case 'Integer':
            return FromInteger(schema_, references_, value);
        case 'Intersect':
            return FromIntersect(schema_, references_, value);
        case 'Iterator':
            return FromIterator(schema_, references_, value);
        case 'Literal':
            return FromLiteral(schema_, references_, value);
        case 'Never':
            return FromNever(schema_, references_, value);
        case 'Not':
            return FromNot(schema_, references_, value);
        case 'Null':
            return FromNull(schema_, references_, value);
        case 'Number':
            return FromNumber(schema_, references_, value);
        case 'Object':
            return FromObject(schema_, references_, value);
        case 'Promise':
            return FromPromise(schema_, references_, value);
        case 'Record':
            return FromRecord(schema_, references_, value);
        case 'Ref':
            return FromRef(schema_, references_, value);
        case 'RegExp':
            return FromRegExp(schema_, references_, value);
        case 'String':
            return FromString(schema_, references_, value);
        case 'Symbol':
            return FromSymbol(schema_, references_, value);
        case 'TemplateLiteral':
            return FromTemplateLiteral(schema_, references_, value);
        case 'This':
            return FromThis(schema_, references_, value);
        case 'Tuple':
            return FromTuple(schema_, references_, value);
        case 'Undefined':
            return FromUndefined(schema_, references_, value);
        case 'Union':
            return FromUnion(schema_, references_, value);
        case 'Uint8Array':
            return FromUint8Array(schema_, references_, value);
        case 'Unknown':
            return FromUnknown(schema_, references_, value);
        case 'Void':
            return FromVoid(schema_, references_, value);
        default:
            if (!index_7.TypeRegistry.Has(schema_[index_4.Kind]))
                throw new ValueCheckUnknownTypeError(schema_);
            return FromKind(schema_, references_, value);
    }
}
/** Returns true if the value matches the given type. */
function Check(...args) {
    return args.length === 3 ? Visit(args[0], args[1], args[2]) : Visit(args[0], [], args[1]);
}
