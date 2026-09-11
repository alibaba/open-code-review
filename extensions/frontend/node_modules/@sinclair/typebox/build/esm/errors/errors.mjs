import { TypeSystemPolicy } from '../system/index.mjs';
import { KeyOfPattern } from '../type/keyof/index.mjs';
import { TypeRegistry, FormatRegistry } from '../type/registry/index.mjs';
import { ExtendsUndefinedCheck } from '../type/extends/extends-undefined.mjs';
import { GetErrorFunction } from './function.mjs';
import { TypeBoxError } from '../type/error/index.mjs';
import { Deref } from '../value/deref/index.mjs';
import { Hash } from '../value/hash/index.mjs';
import { Check } from '../value/check/index.mjs';
import { Kind } from '../type/symbols/index.mjs';
import { Never } from '../type/never/index.mjs';
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
// prettier-ignore
import { IsArray, IsUint8Array, IsDate, IsPromise, IsFunction, IsAsyncIterator, IsIterator, IsBoolean, IsNumber, IsBigInt, IsString, IsSymbol, IsInteger, IsNull, IsUndefined } from '../value/guard/index.mjs';
// ------------------------------------------------------------------
// ValueErrorType
// ------------------------------------------------------------------
export var ValueErrorType;
(function (ValueErrorType) {
    ValueErrorType[ValueErrorType["ArrayContains"] = 0] = "ArrayContains";
    ValueErrorType[ValueErrorType["ArrayMaxContains"] = 1] = "ArrayMaxContains";
    ValueErrorType[ValueErrorType["ArrayMaxItems"] = 2] = "ArrayMaxItems";
    ValueErrorType[ValueErrorType["ArrayMinContains"] = 3] = "ArrayMinContains";
    ValueErrorType[ValueErrorType["ArrayMinItems"] = 4] = "ArrayMinItems";
    ValueErrorType[ValueErrorType["ArrayUniqueItems"] = 5] = "ArrayUniqueItems";
    ValueErrorType[ValueErrorType["Array"] = 6] = "Array";
    ValueErrorType[ValueErrorType["AsyncIterator"] = 7] = "AsyncIterator";
    ValueErrorType[ValueErrorType["BigIntExclusiveMaximum"] = 8] = "BigIntExclusiveMaximum";
    ValueErrorType[ValueErrorType["BigIntExclusiveMinimum"] = 9] = "BigIntExclusiveMinimum";
    ValueErrorType[ValueErrorType["BigIntMaximum"] = 10] = "BigIntMaximum";
    ValueErrorType[ValueErrorType["BigIntMinimum"] = 11] = "BigIntMinimum";
    ValueErrorType[ValueErrorType["BigIntMultipleOf"] = 12] = "BigIntMultipleOf";
    ValueErrorType[ValueErrorType["BigInt"] = 13] = "BigInt";
    ValueErrorType[ValueErrorType["Boolean"] = 14] = "Boolean";
    ValueErrorType[ValueErrorType["DateExclusiveMaximumTimestamp"] = 15] = "DateExclusiveMaximumTimestamp";
    ValueErrorType[ValueErrorType["DateExclusiveMinimumTimestamp"] = 16] = "DateExclusiveMinimumTimestamp";
    ValueErrorType[ValueErrorType["DateMaximumTimestamp"] = 17] = "DateMaximumTimestamp";
    ValueErrorType[ValueErrorType["DateMinimumTimestamp"] = 18] = "DateMinimumTimestamp";
    ValueErrorType[ValueErrorType["DateMultipleOfTimestamp"] = 19] = "DateMultipleOfTimestamp";
    ValueErrorType[ValueErrorType["Date"] = 20] = "Date";
    ValueErrorType[ValueErrorType["Function"] = 21] = "Function";
    ValueErrorType[ValueErrorType["IntegerExclusiveMaximum"] = 22] = "IntegerExclusiveMaximum";
    ValueErrorType[ValueErrorType["IntegerExclusiveMinimum"] = 23] = "IntegerExclusiveMinimum";
    ValueErrorType[ValueErrorType["IntegerMaximum"] = 24] = "IntegerMaximum";
    ValueErrorType[ValueErrorType["IntegerMinimum"] = 25] = "IntegerMinimum";
    ValueErrorType[ValueErrorType["IntegerMultipleOf"] = 26] = "IntegerMultipleOf";
    ValueErrorType[ValueErrorType["Integer"] = 27] = "Integer";
    ValueErrorType[ValueErrorType["IntersectUnevaluatedProperties"] = 28] = "IntersectUnevaluatedProperties";
    ValueErrorType[ValueErrorType["Intersect"] = 29] = "Intersect";
    ValueErrorType[ValueErrorType["Iterator"] = 30] = "Iterator";
    ValueErrorType[ValueErrorType["Kind"] = 31] = "Kind";
    ValueErrorType[ValueErrorType["Literal"] = 32] = "Literal";
    ValueErrorType[ValueErrorType["Never"] = 33] = "Never";
    ValueErrorType[ValueErrorType["Not"] = 34] = "Not";
    ValueErrorType[ValueErrorType["Null"] = 35] = "Null";
    ValueErrorType[ValueErrorType["NumberExclusiveMaximum"] = 36] = "NumberExclusiveMaximum";
    ValueErrorType[ValueErrorType["NumberExclusiveMinimum"] = 37] = "NumberExclusiveMinimum";
    ValueErrorType[ValueErrorType["NumberMaximum"] = 38] = "NumberMaximum";
    ValueErrorType[ValueErrorType["NumberMinimum"] = 39] = "NumberMinimum";
    ValueErrorType[ValueErrorType["NumberMultipleOf"] = 40] = "NumberMultipleOf";
    ValueErrorType[ValueErrorType["Number"] = 41] = "Number";
    ValueErrorType[ValueErrorType["ObjectAdditionalProperties"] = 42] = "ObjectAdditionalProperties";
    ValueErrorType[ValueErrorType["ObjectMaxProperties"] = 43] = "ObjectMaxProperties";
    ValueErrorType[ValueErrorType["ObjectMinProperties"] = 44] = "ObjectMinProperties";
    ValueErrorType[ValueErrorType["ObjectRequiredProperty"] = 45] = "ObjectRequiredProperty";
    ValueErrorType[ValueErrorType["Object"] = 46] = "Object";
    ValueErrorType[ValueErrorType["Promise"] = 47] = "Promise";
    ValueErrorType[ValueErrorType["RegExp"] = 48] = "RegExp";
    ValueErrorType[ValueErrorType["StringFormatUnknown"] = 49] = "StringFormatUnknown";
    ValueErrorType[ValueErrorType["StringFormat"] = 50] = "StringFormat";
    ValueErrorType[ValueErrorType["StringMaxLength"] = 51] = "StringMaxLength";
    ValueErrorType[ValueErrorType["StringMinLength"] = 52] = "StringMinLength";
    ValueErrorType[ValueErrorType["StringPattern"] = 53] = "StringPattern";
    ValueErrorType[ValueErrorType["String"] = 54] = "String";
    ValueErrorType[ValueErrorType["Symbol"] = 55] = "Symbol";
    ValueErrorType[ValueErrorType["TupleLength"] = 56] = "TupleLength";
    ValueErrorType[ValueErrorType["Tuple"] = 57] = "Tuple";
    ValueErrorType[ValueErrorType["Uint8ArrayMaxByteLength"] = 58] = "Uint8ArrayMaxByteLength";
    ValueErrorType[ValueErrorType["Uint8ArrayMinByteLength"] = 59] = "Uint8ArrayMinByteLength";
    ValueErrorType[ValueErrorType["Uint8Array"] = 60] = "Uint8Array";
    ValueErrorType[ValueErrorType["Undefined"] = 61] = "Undefined";
    ValueErrorType[ValueErrorType["Union"] = 62] = "Union";
    ValueErrorType[ValueErrorType["Void"] = 63] = "Void";
})(ValueErrorType || (ValueErrorType = {}));
// ------------------------------------------------------------------
// ValueErrors
// ------------------------------------------------------------------
export class ValueErrorsUnknownTypeError extends TypeBoxError {
    constructor(schema) {
        super('Unknown type');
        this.schema = schema;
    }
}
// ------------------------------------------------------------------
// EscapeKey
// ------------------------------------------------------------------
function EscapeKey(key) {
    return key.replace(/~/g, '~0').replace(/\//g, '~1'); // RFC6901 Path
}
// ------------------------------------------------------------------
// Guards
// ------------------------------------------------------------------
function IsDefined(value) {
    return value !== undefined;
}
// ------------------------------------------------------------------
// ValueErrorIterator
// ------------------------------------------------------------------
export class ValueErrorIterator {
    constructor(iterator) {
        this.iterator = iterator;
    }
    [Symbol.iterator]() {
        return this.iterator;
    }
    /** Returns the first value error or undefined if no errors */
    First() {
        const next = this.iterator.next();
        return next.done ? undefined : next.value;
    }
}
// --------------------------------------------------------------------------
// Create
// --------------------------------------------------------------------------
function Create(errorType, schema, path, value, errors = []) {
    return {
        type: errorType,
        schema,
        path,
        value,
        message: GetErrorFunction()({ errorType, path, schema, value, errors }),
        errors,
    };
}
// --------------------------------------------------------------------------
// Types
// --------------------------------------------------------------------------
function* FromAny(schema, references, path, value) { }
function* FromArgument(schema, references, path, value) { }
function* FromArray(schema, references, path, value) {
    if (!IsArray(value)) {
        return yield Create(ValueErrorType.Array, schema, path, value);
    }
    if (IsDefined(schema.minItems) && !(value.length >= schema.minItems)) {
        yield Create(ValueErrorType.ArrayMinItems, schema, path, value);
    }
    if (IsDefined(schema.maxItems) && !(value.length <= schema.maxItems)) {
        yield Create(ValueErrorType.ArrayMaxItems, schema, path, value);
    }
    for (let i = 0; i < value.length; i++) {
        yield* Visit(schema.items, references, `${path}/${i}`, value[i]);
    }
    // prettier-ignore
    if (schema.uniqueItems === true && !((function () { const set = new Set(); for (const element of value) {
        const hashed = Hash(element);
        if (set.has(hashed)) {
            return false;
        }
        else {
            set.add(hashed);
        }
    } return true; })())) {
        yield Create(ValueErrorType.ArrayUniqueItems, schema, path, value);
    }
    // contains
    if (!(IsDefined(schema.contains) || IsDefined(schema.minContains) || IsDefined(schema.maxContains))) {
        return;
    }
    const containsSchema = IsDefined(schema.contains) ? schema.contains : Never();
    const containsCount = value.reduce((acc, value, index) => (Visit(containsSchema, references, `${path}${index}`, value).next().done === true ? acc + 1 : acc), 0);
    if (containsCount === 0) {
        yield Create(ValueErrorType.ArrayContains, schema, path, value);
    }
    if (IsNumber(schema.minContains) && containsCount < schema.minContains) {
        yield Create(ValueErrorType.ArrayMinContains, schema, path, value);
    }
    if (IsNumber(schema.maxContains) && containsCount > schema.maxContains) {
        yield Create(ValueErrorType.ArrayMaxContains, schema, path, value);
    }
}
function* FromAsyncIterator(schema, references, path, value) {
    if (!IsAsyncIterator(value))
        yield Create(ValueErrorType.AsyncIterator, schema, path, value);
}
function* FromBigInt(schema, references, path, value) {
    if (!IsBigInt(value))
        return yield Create(ValueErrorType.BigInt, schema, path, value);
    if (IsDefined(schema.exclusiveMaximum) && !(value < schema.exclusiveMaximum)) {
        yield Create(ValueErrorType.BigIntExclusiveMaximum, schema, path, value);
    }
    if (IsDefined(schema.exclusiveMinimum) && !(value > schema.exclusiveMinimum)) {
        yield Create(ValueErrorType.BigIntExclusiveMinimum, schema, path, value);
    }
    if (IsDefined(schema.maximum) && !(value <= schema.maximum)) {
        yield Create(ValueErrorType.BigIntMaximum, schema, path, value);
    }
    if (IsDefined(schema.minimum) && !(value >= schema.minimum)) {
        yield Create(ValueErrorType.BigIntMinimum, schema, path, value);
    }
    if (IsDefined(schema.multipleOf) && !(value % schema.multipleOf === BigInt(0))) {
        yield Create(ValueErrorType.BigIntMultipleOf, schema, path, value);
    }
}
function* FromBoolean(schema, references, path, value) {
    if (!IsBoolean(value))
        yield Create(ValueErrorType.Boolean, schema, path, value);
}
function* FromConstructor(schema, references, path, value) {
    yield* Visit(schema.returns, references, path, value.prototype);
}
function* FromDate(schema, references, path, value) {
    if (!IsDate(value))
        return yield Create(ValueErrorType.Date, schema, path, value);
    if (IsDefined(schema.exclusiveMaximumTimestamp) && !(value.getTime() < schema.exclusiveMaximumTimestamp)) {
        yield Create(ValueErrorType.DateExclusiveMaximumTimestamp, schema, path, value);
    }
    if (IsDefined(schema.exclusiveMinimumTimestamp) && !(value.getTime() > schema.exclusiveMinimumTimestamp)) {
        yield Create(ValueErrorType.DateExclusiveMinimumTimestamp, schema, path, value);
    }
    if (IsDefined(schema.maximumTimestamp) && !(value.getTime() <= schema.maximumTimestamp)) {
        yield Create(ValueErrorType.DateMaximumTimestamp, schema, path, value);
    }
    if (IsDefined(schema.minimumTimestamp) && !(value.getTime() >= schema.minimumTimestamp)) {
        yield Create(ValueErrorType.DateMinimumTimestamp, schema, path, value);
    }
    if (IsDefined(schema.multipleOfTimestamp) && !(value.getTime() % schema.multipleOfTimestamp === 0)) {
        yield Create(ValueErrorType.DateMultipleOfTimestamp, schema, path, value);
    }
}
function* FromFunction(schema, references, path, value) {
    if (!IsFunction(value))
        yield Create(ValueErrorType.Function, schema, path, value);
}
function* FromImport(schema, references, path, value) {
    const definitions = globalThis.Object.values(schema.$defs);
    const target = schema.$defs[schema.$ref];
    yield* Visit(target, [...references, ...definitions], path, value);
}
function* FromInteger(schema, references, path, value) {
    if (!IsInteger(value))
        return yield Create(ValueErrorType.Integer, schema, path, value);
    if (IsDefined(schema.exclusiveMaximum) && !(value < schema.exclusiveMaximum)) {
        yield Create(ValueErrorType.IntegerExclusiveMaximum, schema, path, value);
    }
    if (IsDefined(schema.exclusiveMinimum) && !(value > schema.exclusiveMinimum)) {
        yield Create(ValueErrorType.IntegerExclusiveMinimum, schema, path, value);
    }
    if (IsDefined(schema.maximum) && !(value <= schema.maximum)) {
        yield Create(ValueErrorType.IntegerMaximum, schema, path, value);
    }
    if (IsDefined(schema.minimum) && !(value >= schema.minimum)) {
        yield Create(ValueErrorType.IntegerMinimum, schema, path, value);
    }
    if (IsDefined(schema.multipleOf) && !(value % schema.multipleOf === 0)) {
        yield Create(ValueErrorType.IntegerMultipleOf, schema, path, value);
    }
}
function* FromIntersect(schema, references, path, value) {
    let hasError = false;
    for (const inner of schema.allOf) {
        for (const error of Visit(inner, references, path, value)) {
            hasError = true;
            yield error;
        }
    }
    if (hasError) {
        return yield Create(ValueErrorType.Intersect, schema, path, value);
    }
    if (schema.unevaluatedProperties === false) {
        const keyCheck = new RegExp(KeyOfPattern(schema));
        for (const valueKey of Object.getOwnPropertyNames(value)) {
            if (!keyCheck.test(valueKey)) {
                yield Create(ValueErrorType.IntersectUnevaluatedProperties, schema, `${path}/${valueKey}`, value);
            }
        }
    }
    if (typeof schema.unevaluatedProperties === 'object') {
        const keyCheck = new RegExp(KeyOfPattern(schema));
        for (const valueKey of Object.getOwnPropertyNames(value)) {
            if (!keyCheck.test(valueKey)) {
                const next = Visit(schema.unevaluatedProperties, references, `${path}/${valueKey}`, value[valueKey]).next();
                if (!next.done)
                    yield next.value; // yield interior
            }
        }
    }
}
function* FromIterator(schema, references, path, value) {
    if (!IsIterator(value))
        yield Create(ValueErrorType.Iterator, schema, path, value);
}
function* FromLiteral(schema, references, path, value) {
    if (!(value === schema.const))
        yield Create(ValueErrorType.Literal, schema, path, value);
}
function* FromNever(schema, references, path, value) {
    yield Create(ValueErrorType.Never, schema, path, value);
}
function* FromNot(schema, references, path, value) {
    if (Visit(schema.not, references, path, value).next().done === true)
        yield Create(ValueErrorType.Not, schema, path, value);
}
function* FromNull(schema, references, path, value) {
    if (!IsNull(value))
        yield Create(ValueErrorType.Null, schema, path, value);
}
function* FromNumber(schema, references, path, value) {
    if (!TypeSystemPolicy.IsNumberLike(value))
        return yield Create(ValueErrorType.Number, schema, path, value);
    if (IsDefined(schema.exclusiveMaximum) && !(value < schema.exclusiveMaximum)) {
        yield Create(ValueErrorType.NumberExclusiveMaximum, schema, path, value);
    }
    if (IsDefined(schema.exclusiveMinimum) && !(value > schema.exclusiveMinimum)) {
        yield Create(ValueErrorType.NumberExclusiveMinimum, schema, path, value);
    }
    if (IsDefined(schema.maximum) && !(value <= schema.maximum)) {
        yield Create(ValueErrorType.NumberMaximum, schema, path, value);
    }
    if (IsDefined(schema.minimum) && !(value >= schema.minimum)) {
        yield Create(ValueErrorType.NumberMinimum, schema, path, value);
    }
    if (IsDefined(schema.multipleOf) && !(value % schema.multipleOf === 0)) {
        yield Create(ValueErrorType.NumberMultipleOf, schema, path, value);
    }
}
function* FromObject(schema, references, path, value) {
    if (!TypeSystemPolicy.IsObjectLike(value))
        return yield Create(ValueErrorType.Object, schema, path, value);
    if (IsDefined(schema.minProperties) && !(Object.getOwnPropertyNames(value).length >= schema.minProperties)) {
        yield Create(ValueErrorType.ObjectMinProperties, schema, path, value);
    }
    if (IsDefined(schema.maxProperties) && !(Object.getOwnPropertyNames(value).length <= schema.maxProperties)) {
        yield Create(ValueErrorType.ObjectMaxProperties, schema, path, value);
    }
    const requiredKeys = Array.isArray(schema.required) ? schema.required : [];
    const knownKeys = Object.getOwnPropertyNames(schema.properties);
    const unknownKeys = Object.getOwnPropertyNames(value);
    for (const requiredKey of requiredKeys) {
        if (unknownKeys.includes(requiredKey))
            continue;
        yield Create(ValueErrorType.ObjectRequiredProperty, schema.properties[requiredKey], `${path}/${EscapeKey(requiredKey)}`, undefined);
    }
    if (schema.additionalProperties === false) {
        for (const valueKey of unknownKeys) {
            if (!knownKeys.includes(valueKey)) {
                yield Create(ValueErrorType.ObjectAdditionalProperties, schema, `${path}/${EscapeKey(valueKey)}`, value[valueKey]);
            }
        }
    }
    if (typeof schema.additionalProperties === 'object') {
        for (const valueKey of unknownKeys) {
            if (knownKeys.includes(valueKey))
                continue;
            yield* Visit(schema.additionalProperties, references, `${path}/${EscapeKey(valueKey)}`, value[valueKey]);
        }
    }
    for (const knownKey of knownKeys) {
        const property = schema.properties[knownKey];
        if (schema.required && schema.required.includes(knownKey)) {
            yield* Visit(property, references, `${path}/${EscapeKey(knownKey)}`, value[knownKey]);
            if (ExtendsUndefinedCheck(schema) && !(knownKey in value)) {
                yield Create(ValueErrorType.ObjectRequiredProperty, property, `${path}/${EscapeKey(knownKey)}`, undefined);
            }
        }
        else {
            if (TypeSystemPolicy.IsExactOptionalProperty(value, knownKey)) {
                yield* Visit(property, references, `${path}/${EscapeKey(knownKey)}`, value[knownKey]);
            }
        }
    }
}
function* FromPromise(schema, references, path, value) {
    if (!IsPromise(value))
        yield Create(ValueErrorType.Promise, schema, path, value);
}
function* FromRecord(schema, references, path, value) {
    if (!TypeSystemPolicy.IsRecordLike(value))
        return yield Create(ValueErrorType.Object, schema, path, value);
    if (IsDefined(schema.minProperties) && !(Object.getOwnPropertyNames(value).length >= schema.minProperties)) {
        yield Create(ValueErrorType.ObjectMinProperties, schema, path, value);
    }
    if (IsDefined(schema.maxProperties) && !(Object.getOwnPropertyNames(value).length <= schema.maxProperties)) {
        yield Create(ValueErrorType.ObjectMaxProperties, schema, path, value);
    }
    const [patternKey, patternSchema] = Object.entries(schema.patternProperties)[0];
    const regex = new RegExp(patternKey);
    for (const [propertyKey, propertyValue] of Object.entries(value)) {
        if (regex.test(propertyKey))
            yield* Visit(patternSchema, references, `${path}/${EscapeKey(propertyKey)}`, propertyValue);
    }
    if (typeof schema.additionalProperties === 'object') {
        for (const [propertyKey, propertyValue] of Object.entries(value)) {
            if (!regex.test(propertyKey))
                yield* Visit(schema.additionalProperties, references, `${path}/${EscapeKey(propertyKey)}`, propertyValue);
        }
    }
    if (schema.additionalProperties === false) {
        for (const [propertyKey, propertyValue] of Object.entries(value)) {
            if (regex.test(propertyKey))
                continue;
            return yield Create(ValueErrorType.ObjectAdditionalProperties, schema, `${path}/${EscapeKey(propertyKey)}`, propertyValue);
        }
    }
}
function* FromRef(schema, references, path, value) {
    yield* Visit(Deref(schema, references), references, path, value);
}
function* FromRegExp(schema, references, path, value) {
    if (!IsString(value))
        return yield Create(ValueErrorType.String, schema, path, value);
    if (IsDefined(schema.minLength) && !(value.length >= schema.minLength)) {
        yield Create(ValueErrorType.StringMinLength, schema, path, value);
    }
    if (IsDefined(schema.maxLength) && !(value.length <= schema.maxLength)) {
        yield Create(ValueErrorType.StringMaxLength, schema, path, value);
    }
    const regex = new RegExp(schema.source, schema.flags);
    if (!regex.test(value)) {
        return yield Create(ValueErrorType.RegExp, schema, path, value);
    }
}
function* FromString(schema, references, path, value) {
    if (!IsString(value))
        return yield Create(ValueErrorType.String, schema, path, value);
    if (IsDefined(schema.minLength) && !(value.length >= schema.minLength)) {
        yield Create(ValueErrorType.StringMinLength, schema, path, value);
    }
    if (IsDefined(schema.maxLength) && !(value.length <= schema.maxLength)) {
        yield Create(ValueErrorType.StringMaxLength, schema, path, value);
    }
    if (IsString(schema.pattern)) {
        const regex = new RegExp(schema.pattern);
        if (!regex.test(value)) {
            yield Create(ValueErrorType.StringPattern, schema, path, value);
        }
    }
    if (IsString(schema.format)) {
        if (!FormatRegistry.Has(schema.format)) {
            yield Create(ValueErrorType.StringFormatUnknown, schema, path, value);
        }
        else {
            const format = FormatRegistry.Get(schema.format);
            if (!format(value)) {
                yield Create(ValueErrorType.StringFormat, schema, path, value);
            }
        }
    }
}
function* FromSymbol(schema, references, path, value) {
    if (!IsSymbol(value))
        yield Create(ValueErrorType.Symbol, schema, path, value);
}
function* FromTemplateLiteral(schema, references, path, value) {
    if (!IsString(value))
        return yield Create(ValueErrorType.String, schema, path, value);
    const regex = new RegExp(schema.pattern);
    if (!regex.test(value)) {
        yield Create(ValueErrorType.StringPattern, schema, path, value);
    }
}
function* FromThis(schema, references, path, value) {
    yield* Visit(Deref(schema, references), references, path, value);
}
function* FromTuple(schema, references, path, value) {
    if (!IsArray(value))
        return yield Create(ValueErrorType.Tuple, schema, path, value);
    if (schema.items === undefined && !(value.length === 0)) {
        return yield Create(ValueErrorType.TupleLength, schema, path, value);
    }
    if (!(value.length === schema.maxItems)) {
        return yield Create(ValueErrorType.TupleLength, schema, path, value);
    }
    if (!schema.items) {
        return;
    }
    for (let i = 0; i < schema.items.length; i++) {
        yield* Visit(schema.items[i], references, `${path}/${i}`, value[i]);
    }
}
function* FromUndefined(schema, references, path, value) {
    if (!IsUndefined(value))
        yield Create(ValueErrorType.Undefined, schema, path, value);
}
function* FromUnion(schema, references, path, value) {
    if (Check(schema, references, value))
        return;
    const errors = schema.anyOf.map((variant) => new ValueErrorIterator(Visit(variant, references, path, value)));
    yield Create(ValueErrorType.Union, schema, path, value, errors);
}
function* FromUint8Array(schema, references, path, value) {
    if (!IsUint8Array(value))
        return yield Create(ValueErrorType.Uint8Array, schema, path, value);
    if (IsDefined(schema.maxByteLength) && !(value.length <= schema.maxByteLength)) {
        yield Create(ValueErrorType.Uint8ArrayMaxByteLength, schema, path, value);
    }
    if (IsDefined(schema.minByteLength) && !(value.length >= schema.minByteLength)) {
        yield Create(ValueErrorType.Uint8ArrayMinByteLength, schema, path, value);
    }
}
function* FromUnknown(schema, references, path, value) { }
function* FromVoid(schema, references, path, value) {
    if (!TypeSystemPolicy.IsVoidLike(value))
        yield Create(ValueErrorType.Void, schema, path, value);
}
function* FromKind(schema, references, path, value) {
    const check = TypeRegistry.Get(schema[Kind]);
    if (!check(schema, value))
        yield Create(ValueErrorType.Kind, schema, path, value);
}
function* Visit(schema, references, path, value) {
    const references_ = IsDefined(schema.$id) ? [...references, schema] : references;
    const schema_ = schema;
    switch (schema_[Kind]) {
        case 'Any':
            return yield* FromAny(schema_, references_, path, value);
        case 'Argument':
            return yield* FromArgument(schema_, references_, path, value);
        case 'Array':
            return yield* FromArray(schema_, references_, path, value);
        case 'AsyncIterator':
            return yield* FromAsyncIterator(schema_, references_, path, value);
        case 'BigInt':
            return yield* FromBigInt(schema_, references_, path, value);
        case 'Boolean':
            return yield* FromBoolean(schema_, references_, path, value);
        case 'Constructor':
            return yield* FromConstructor(schema_, references_, path, value);
        case 'Date':
            return yield* FromDate(schema_, references_, path, value);
        case 'Function':
            return yield* FromFunction(schema_, references_, path, value);
        case 'Import':
            return yield* FromImport(schema_, references_, path, value);
        case 'Integer':
            return yield* FromInteger(schema_, references_, path, value);
        case 'Intersect':
            return yield* FromIntersect(schema_, references_, path, value);
        case 'Iterator':
            return yield* FromIterator(schema_, references_, path, value);
        case 'Literal':
            return yield* FromLiteral(schema_, references_, path, value);
        case 'Never':
            return yield* FromNever(schema_, references_, path, value);
        case 'Not':
            return yield* FromNot(schema_, references_, path, value);
        case 'Null':
            return yield* FromNull(schema_, references_, path, value);
        case 'Number':
            return yield* FromNumber(schema_, references_, path, value);
        case 'Object':
            return yield* FromObject(schema_, references_, path, value);
        case 'Promise':
            return yield* FromPromise(schema_, references_, path, value);
        case 'Record':
            return yield* FromRecord(schema_, references_, path, value);
        case 'Ref':
            return yield* FromRef(schema_, references_, path, value);
        case 'RegExp':
            return yield* FromRegExp(schema_, references_, path, value);
        case 'String':
            return yield* FromString(schema_, references_, path, value);
        case 'Symbol':
            return yield* FromSymbol(schema_, references_, path, value);
        case 'TemplateLiteral':
            return yield* FromTemplateLiteral(schema_, references_, path, value);
        case 'This':
            return yield* FromThis(schema_, references_, path, value);
        case 'Tuple':
            return yield* FromTuple(schema_, references_, path, value);
        case 'Undefined':
            return yield* FromUndefined(schema_, references_, path, value);
        case 'Union':
            return yield* FromUnion(schema_, references_, path, value);
        case 'Uint8Array':
            return yield* FromUint8Array(schema_, references_, path, value);
        case 'Unknown':
            return yield* FromUnknown(schema_, references_, path, value);
        case 'Void':
            return yield* FromVoid(schema_, references_, path, value);
        default:
            if (!TypeRegistry.Has(schema_[Kind]))
                throw new ValueErrorsUnknownTypeError(schema);
            return yield* FromKind(schema_, references_, path, value);
    }
}
/** Returns an iterator for each error in this value. */
export function Errors(...args) {
    const iterator = args.length === 3 ? Visit(args[0], args[1], '', args[2]) : Visit(args[0], [], '', args[1]);
    return new ValueErrorIterator(iterator);
}
