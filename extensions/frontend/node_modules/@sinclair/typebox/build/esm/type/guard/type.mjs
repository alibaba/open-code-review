import * as ValueGuard from './value.mjs';
import { Kind, Hint, TransformKind, ReadonlyKind, OptionalKind } from '../symbols/index.mjs';
import { TypeBoxError } from '../error/index.mjs';
export class TypeGuardUnknownTypeError extends TypeBoxError {
}
const KnownTypes = [
    'Argument',
    'Any',
    'Array',
    'AsyncIterator',
    'BigInt',
    'Boolean',
    'Computed',
    'Constructor',
    'Date',
    'Enum',
    'Function',
    'Integer',
    'Intersect',
    'Iterator',
    'Literal',
    'MappedKey',
    'MappedResult',
    'Not',
    'Null',
    'Number',
    'Object',
    'Promise',
    'Record',
    'Ref',
    'RegExp',
    'String',
    'Symbol',
    'TemplateLiteral',
    'This',
    'Tuple',
    'Undefined',
    'Union',
    'Uint8Array',
    'Unknown',
    'Void',
];
function IsPattern(value) {
    try {
        new RegExp(value);
        return true;
    }
    catch {
        return false;
    }
}
function IsControlCharacterFree(value) {
    if (!ValueGuard.IsString(value))
        return false;
    for (let i = 0; i < value.length; i++) {
        const code = value.charCodeAt(i);
        if ((code >= 7 && code <= 13) || code === 27 || code === 127) {
            return false;
        }
    }
    return true;
}
function IsAdditionalProperties(value) {
    return IsOptionalBoolean(value) || IsSchema(value);
}
function IsOptionalBigInt(value) {
    return ValueGuard.IsUndefined(value) || ValueGuard.IsBigInt(value);
}
function IsOptionalNumber(value) {
    return ValueGuard.IsUndefined(value) || ValueGuard.IsNumber(value);
}
function IsOptionalBoolean(value) {
    return ValueGuard.IsUndefined(value) || ValueGuard.IsBoolean(value);
}
function IsOptionalString(value) {
    return ValueGuard.IsUndefined(value) || ValueGuard.IsString(value);
}
function IsOptionalPattern(value) {
    return ValueGuard.IsUndefined(value) || (ValueGuard.IsString(value) && IsControlCharacterFree(value) && IsPattern(value));
}
function IsOptionalFormat(value) {
    return ValueGuard.IsUndefined(value) || (ValueGuard.IsString(value) && IsControlCharacterFree(value));
}
function IsOptionalSchema(value) {
    return ValueGuard.IsUndefined(value) || IsSchema(value);
}
// ------------------------------------------------------------------
// Modifiers
// ------------------------------------------------------------------
/** Returns true if this value has a Readonly symbol */
export function IsReadonly(value) {
    return ValueGuard.IsObject(value) && value[ReadonlyKind] === 'Readonly';
}
/** Returns true if this value has a Optional symbol */
export function IsOptional(value) {
    return ValueGuard.IsObject(value) && value[OptionalKind] === 'Optional';
}
// ------------------------------------------------------------------
// Types
// ------------------------------------------------------------------
/** Returns true if the given value is TAny */
export function IsAny(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Any') &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TArgument */
export function IsArgument(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Argument') &&
        ValueGuard.IsNumber(value.index));
}
/** Returns true if the given value is TArray */
export function IsArray(value) {
    return (IsKindOf(value, 'Array') &&
        value.type === 'array' &&
        IsOptionalString(value.$id) &&
        IsSchema(value.items) &&
        IsOptionalNumber(value.minItems) &&
        IsOptionalNumber(value.maxItems) &&
        IsOptionalBoolean(value.uniqueItems) &&
        IsOptionalSchema(value.contains) &&
        IsOptionalNumber(value.minContains) &&
        IsOptionalNumber(value.maxContains));
}
/** Returns true if the given value is TAsyncIterator */
export function IsAsyncIterator(value) {
    // prettier-ignore
    return (IsKindOf(value, 'AsyncIterator') &&
        value.type === 'AsyncIterator' &&
        IsOptionalString(value.$id) &&
        IsSchema(value.items));
}
/** Returns true if the given value is TBigInt */
export function IsBigInt(value) {
    // prettier-ignore
    return (IsKindOf(value, 'BigInt') &&
        value.type === 'bigint' &&
        IsOptionalString(value.$id) &&
        IsOptionalBigInt(value.exclusiveMaximum) &&
        IsOptionalBigInt(value.exclusiveMinimum) &&
        IsOptionalBigInt(value.maximum) &&
        IsOptionalBigInt(value.minimum) &&
        IsOptionalBigInt(value.multipleOf));
}
/** Returns true if the given value is TBoolean */
export function IsBoolean(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Boolean') &&
        value.type === 'boolean' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TComputed */
export function IsComputed(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Computed') &&
        ValueGuard.IsString(value.target) &&
        ValueGuard.IsArray(value.parameters) &&
        value.parameters.every((schema) => IsSchema(schema)));
}
/** Returns true if the given value is TConstructor */
export function IsConstructor(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Constructor') &&
        value.type === 'Constructor' &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsArray(value.parameters) &&
        value.parameters.every(schema => IsSchema(schema)) &&
        IsSchema(value.returns));
}
/** Returns true if the given value is TDate */
export function IsDate(value) {
    return (IsKindOf(value, 'Date') &&
        value.type === 'Date' &&
        IsOptionalString(value.$id) &&
        IsOptionalNumber(value.exclusiveMaximumTimestamp) &&
        IsOptionalNumber(value.exclusiveMinimumTimestamp) &&
        IsOptionalNumber(value.maximumTimestamp) &&
        IsOptionalNumber(value.minimumTimestamp) &&
        IsOptionalNumber(value.multipleOfTimestamp));
}
/** Returns true if the given value is TFunction */
export function IsFunction(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Function') &&
        value.type === 'Function' &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsArray(value.parameters) &&
        value.parameters.every(schema => IsSchema(schema)) &&
        IsSchema(value.returns));
}
/** Returns true if the given value is TImport */
export function IsImport(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Import') &&
        ValueGuard.HasPropertyKey(value, '$defs') &&
        ValueGuard.IsObject(value.$defs) &&
        IsProperties(value.$defs) &&
        ValueGuard.HasPropertyKey(value, '$ref') &&
        ValueGuard.IsString(value.$ref) &&
        value.$ref in value.$defs // required
    );
}
/** Returns true if the given value is TInteger */
export function IsInteger(value) {
    return (IsKindOf(value, 'Integer') &&
        value.type === 'integer' &&
        IsOptionalString(value.$id) &&
        IsOptionalNumber(value.exclusiveMaximum) &&
        IsOptionalNumber(value.exclusiveMinimum) &&
        IsOptionalNumber(value.maximum) &&
        IsOptionalNumber(value.minimum) &&
        IsOptionalNumber(value.multipleOf));
}
/** Returns true if the given schema is TProperties */
export function IsProperties(value) {
    // prettier-ignore
    return (ValueGuard.IsObject(value) &&
        Object.entries(value).every(([key, schema]) => IsControlCharacterFree(key) && IsSchema(schema)));
}
/** Returns true if the given value is TIntersect */
export function IsIntersect(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Intersect') &&
        (ValueGuard.IsString(value.type) && value.type !== 'object' ? false : true) &&
        ValueGuard.IsArray(value.allOf) &&
        value.allOf.every(schema => IsSchema(schema) && !IsTransform(schema)) &&
        IsOptionalString(value.type) &&
        (IsOptionalBoolean(value.unevaluatedProperties) || IsOptionalSchema(value.unevaluatedProperties)) &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TIterator */
export function IsIterator(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Iterator') &&
        value.type === 'Iterator' &&
        IsOptionalString(value.$id) &&
        IsSchema(value.items));
}
/** Returns true if the given value is a TKind with the given name. */
export function IsKindOf(value, kind) {
    return ValueGuard.IsObject(value) && Kind in value && value[Kind] === kind;
}
/** Returns true if the given value is TLiteral<string> */
export function IsLiteralString(value) {
    return IsLiteral(value) && ValueGuard.IsString(value.const);
}
/** Returns true if the given value is TLiteral<number> */
export function IsLiteralNumber(value) {
    return IsLiteral(value) && ValueGuard.IsNumber(value.const);
}
/** Returns true if the given value is TLiteral<boolean> */
export function IsLiteralBoolean(value) {
    return IsLiteral(value) && ValueGuard.IsBoolean(value.const);
}
/** Returns true if the given value is TLiteral */
export function IsLiteral(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Literal') &&
        IsOptionalString(value.$id) && IsLiteralValue(value.const));
}
/** Returns true if the given value is a TLiteralValue */
export function IsLiteralValue(value) {
    return ValueGuard.IsBoolean(value) || ValueGuard.IsNumber(value) || ValueGuard.IsString(value);
}
/** Returns true if the given value is a TMappedKey */
export function IsMappedKey(value) {
    // prettier-ignore
    return (IsKindOf(value, 'MappedKey') &&
        ValueGuard.IsArray(value.keys) &&
        value.keys.every(key => ValueGuard.IsNumber(key) || ValueGuard.IsString(key)));
}
/** Returns true if the given value is TMappedResult */
export function IsMappedResult(value) {
    // prettier-ignore
    return (IsKindOf(value, 'MappedResult') &&
        IsProperties(value.properties));
}
/** Returns true if the given value is TNever */
export function IsNever(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Never') &&
        ValueGuard.IsObject(value.not) &&
        Object.getOwnPropertyNames(value.not).length === 0);
}
/** Returns true if the given value is TNot */
export function IsNot(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Not') &&
        IsSchema(value.not));
}
/** Returns true if the given value is TNull */
export function IsNull(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Null') &&
        value.type === 'null' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TNumber */
export function IsNumber(value) {
    return (IsKindOf(value, 'Number') &&
        value.type === 'number' &&
        IsOptionalString(value.$id) &&
        IsOptionalNumber(value.exclusiveMaximum) &&
        IsOptionalNumber(value.exclusiveMinimum) &&
        IsOptionalNumber(value.maximum) &&
        IsOptionalNumber(value.minimum) &&
        IsOptionalNumber(value.multipleOf));
}
/** Returns true if the given value is TObject */
export function IsObject(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Object') &&
        value.type === 'object' &&
        IsOptionalString(value.$id) &&
        IsProperties(value.properties) &&
        IsAdditionalProperties(value.additionalProperties) &&
        IsOptionalNumber(value.minProperties) &&
        IsOptionalNumber(value.maxProperties));
}
/** Returns true if the given value is TPromise */
export function IsPromise(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Promise') &&
        value.type === 'Promise' &&
        IsOptionalString(value.$id) &&
        IsSchema(value.item));
}
/** Returns true if the given value is TRecord */
export function IsRecord(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Record') &&
        value.type === 'object' &&
        IsOptionalString(value.$id) &&
        IsAdditionalProperties(value.additionalProperties) &&
        ValueGuard.IsObject(value.patternProperties) &&
        ((schema) => {
            const keys = Object.getOwnPropertyNames(schema.patternProperties);
            return (keys.length === 1 &&
                IsPattern(keys[0]) &&
                ValueGuard.IsObject(schema.patternProperties) &&
                IsSchema(schema.patternProperties[keys[0]]));
        })(value));
}
/** Returns true if this value is TRecursive */
export function IsRecursive(value) {
    return ValueGuard.IsObject(value) && Hint in value && value[Hint] === 'Recursive';
}
/** Returns true if the given value is TRef */
export function IsRef(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Ref') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsString(value.$ref));
}
/** Returns true if the given value is TRegExp */
export function IsRegExp(value) {
    // prettier-ignore
    return (IsKindOf(value, 'RegExp') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsString(value.source) &&
        ValueGuard.IsString(value.flags) &&
        IsOptionalNumber(value.maxLength) &&
        IsOptionalNumber(value.minLength));
}
/** Returns true if the given value is TString */
export function IsString(value) {
    // prettier-ignore
    return (IsKindOf(value, 'String') &&
        value.type === 'string' &&
        IsOptionalString(value.$id) &&
        IsOptionalNumber(value.minLength) &&
        IsOptionalNumber(value.maxLength) &&
        IsOptionalPattern(value.pattern) &&
        IsOptionalFormat(value.format));
}
/** Returns true if the given value is TSymbol */
export function IsSymbol(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Symbol') &&
        value.type === 'symbol' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TTemplateLiteral */
export function IsTemplateLiteral(value) {
    // prettier-ignore
    return (IsKindOf(value, 'TemplateLiteral') &&
        value.type === 'string' &&
        ValueGuard.IsString(value.pattern) &&
        value.pattern[0] === '^' &&
        value.pattern[value.pattern.length - 1] === '$');
}
/** Returns true if the given value is TThis */
export function IsThis(value) {
    // prettier-ignore
    return (IsKindOf(value, 'This') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsString(value.$ref));
}
/** Returns true of this value is TTransform */
export function IsTransform(value) {
    return ValueGuard.IsObject(value) && TransformKind in value;
}
/** Returns true if the given value is TTuple */
export function IsTuple(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Tuple') &&
        value.type === 'array' &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsNumber(value.minItems) &&
        ValueGuard.IsNumber(value.maxItems) &&
        value.minItems === value.maxItems &&
        (( // empty
        ValueGuard.IsUndefined(value.items) &&
            ValueGuard.IsUndefined(value.additionalItems) &&
            value.minItems === 0) || (ValueGuard.IsArray(value.items) &&
            value.items.every(schema => IsSchema(schema)))));
}
/** Returns true if the given value is TUndefined */
export function IsUndefined(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Undefined') &&
        value.type === 'undefined' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TUnion<Literal<string | number>[]> */
export function IsUnionLiteral(value) {
    return IsUnion(value) && value.anyOf.every((schema) => IsLiteralString(schema) || IsLiteralNumber(schema));
}
/** Returns true if the given value is TUnion */
export function IsUnion(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Union') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsObject(value) &&
        ValueGuard.IsArray(value.anyOf) &&
        value.anyOf.every(schema => IsSchema(schema)));
}
/** Returns true if the given value is TUint8Array */
export function IsUint8Array(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Uint8Array') &&
        value.type === 'Uint8Array' &&
        IsOptionalString(value.$id) &&
        IsOptionalNumber(value.minByteLength) &&
        IsOptionalNumber(value.maxByteLength));
}
/** Returns true if the given value is TUnknown */
export function IsUnknown(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Unknown') &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is a raw TUnsafe */
export function IsUnsafe(value) {
    return IsKindOf(value, 'Unsafe');
}
/** Returns true if the given value is TVoid */
export function IsVoid(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Void') &&
        value.type === 'void' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TKind */
export function IsKind(value) {
    return ValueGuard.IsObject(value) && Kind in value && ValueGuard.IsString(value[Kind]) && !KnownTypes.includes(value[Kind]);
}
/** Returns true if the given value is TSchema */
export function IsSchema(value) {
    // prettier-ignore
    return (ValueGuard.IsObject(value)) && (IsAny(value) ||
        IsArgument(value) ||
        IsArray(value) ||
        IsBoolean(value) ||
        IsBigInt(value) ||
        IsAsyncIterator(value) ||
        IsComputed(value) ||
        IsConstructor(value) ||
        IsDate(value) ||
        IsFunction(value) ||
        IsInteger(value) ||
        IsIntersect(value) ||
        IsIterator(value) ||
        IsLiteral(value) ||
        IsMappedKey(value) ||
        IsMappedResult(value) ||
        IsNever(value) ||
        IsNot(value) ||
        IsNull(value) ||
        IsNumber(value) ||
        IsObject(value) ||
        IsPromise(value) ||
        IsRecord(value) ||
        IsRef(value) ||
        IsRegExp(value) ||
        IsString(value) ||
        IsSymbol(value) ||
        IsTemplateLiteral(value) ||
        IsThis(value) ||
        IsTuple(value) ||
        IsUndefined(value) ||
        IsUnion(value) ||
        IsUint8Array(value) ||
        IsUnknown(value) ||
        IsUnsafe(value) ||
        IsVoid(value) ||
        IsKind(value));
}
