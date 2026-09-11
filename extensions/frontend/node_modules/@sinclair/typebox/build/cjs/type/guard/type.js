"use strict";

var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.TypeGuardUnknownTypeError = void 0;
exports.IsReadonly = IsReadonly;
exports.IsOptional = IsOptional;
exports.IsAny = IsAny;
exports.IsArgument = IsArgument;
exports.IsArray = IsArray;
exports.IsAsyncIterator = IsAsyncIterator;
exports.IsBigInt = IsBigInt;
exports.IsBoolean = IsBoolean;
exports.IsComputed = IsComputed;
exports.IsConstructor = IsConstructor;
exports.IsDate = IsDate;
exports.IsFunction = IsFunction;
exports.IsImport = IsImport;
exports.IsInteger = IsInteger;
exports.IsProperties = IsProperties;
exports.IsIntersect = IsIntersect;
exports.IsIterator = IsIterator;
exports.IsKindOf = IsKindOf;
exports.IsLiteralString = IsLiteralString;
exports.IsLiteralNumber = IsLiteralNumber;
exports.IsLiteralBoolean = IsLiteralBoolean;
exports.IsLiteral = IsLiteral;
exports.IsLiteralValue = IsLiteralValue;
exports.IsMappedKey = IsMappedKey;
exports.IsMappedResult = IsMappedResult;
exports.IsNever = IsNever;
exports.IsNot = IsNot;
exports.IsNull = IsNull;
exports.IsNumber = IsNumber;
exports.IsObject = IsObject;
exports.IsPromise = IsPromise;
exports.IsRecord = IsRecord;
exports.IsRecursive = IsRecursive;
exports.IsRef = IsRef;
exports.IsRegExp = IsRegExp;
exports.IsString = IsString;
exports.IsSymbol = IsSymbol;
exports.IsTemplateLiteral = IsTemplateLiteral;
exports.IsThis = IsThis;
exports.IsTransform = IsTransform;
exports.IsTuple = IsTuple;
exports.IsUndefined = IsUndefined;
exports.IsUnionLiteral = IsUnionLiteral;
exports.IsUnion = IsUnion;
exports.IsUint8Array = IsUint8Array;
exports.IsUnknown = IsUnknown;
exports.IsUnsafe = IsUnsafe;
exports.IsVoid = IsVoid;
exports.IsKind = IsKind;
exports.IsSchema = IsSchema;
const ValueGuard = __importStar(require("./value"));
const index_1 = require("../symbols/index");
const index_2 = require("../error/index");
class TypeGuardUnknownTypeError extends index_2.TypeBoxError {
}
exports.TypeGuardUnknownTypeError = TypeGuardUnknownTypeError;
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
function IsReadonly(value) {
    return ValueGuard.IsObject(value) && value[index_1.ReadonlyKind] === 'Readonly';
}
/** Returns true if this value has a Optional symbol */
function IsOptional(value) {
    return ValueGuard.IsObject(value) && value[index_1.OptionalKind] === 'Optional';
}
// ------------------------------------------------------------------
// Types
// ------------------------------------------------------------------
/** Returns true if the given value is TAny */
function IsAny(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Any') &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TArgument */
function IsArgument(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Argument') &&
        ValueGuard.IsNumber(value.index));
}
/** Returns true if the given value is TArray */
function IsArray(value) {
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
function IsAsyncIterator(value) {
    // prettier-ignore
    return (IsKindOf(value, 'AsyncIterator') &&
        value.type === 'AsyncIterator' &&
        IsOptionalString(value.$id) &&
        IsSchema(value.items));
}
/** Returns true if the given value is TBigInt */
function IsBigInt(value) {
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
function IsBoolean(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Boolean') &&
        value.type === 'boolean' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TComputed */
function IsComputed(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Computed') &&
        ValueGuard.IsString(value.target) &&
        ValueGuard.IsArray(value.parameters) &&
        value.parameters.every((schema) => IsSchema(schema)));
}
/** Returns true if the given value is TConstructor */
function IsConstructor(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Constructor') &&
        value.type === 'Constructor' &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsArray(value.parameters) &&
        value.parameters.every(schema => IsSchema(schema)) &&
        IsSchema(value.returns));
}
/** Returns true if the given value is TDate */
function IsDate(value) {
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
function IsFunction(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Function') &&
        value.type === 'Function' &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsArray(value.parameters) &&
        value.parameters.every(schema => IsSchema(schema)) &&
        IsSchema(value.returns));
}
/** Returns true if the given value is TImport */
function IsImport(value) {
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
function IsInteger(value) {
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
function IsProperties(value) {
    // prettier-ignore
    return (ValueGuard.IsObject(value) &&
        Object.entries(value).every(([key, schema]) => IsControlCharacterFree(key) && IsSchema(schema)));
}
/** Returns true if the given value is TIntersect */
function IsIntersect(value) {
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
function IsIterator(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Iterator') &&
        value.type === 'Iterator' &&
        IsOptionalString(value.$id) &&
        IsSchema(value.items));
}
/** Returns true if the given value is a TKind with the given name. */
function IsKindOf(value, kind) {
    return ValueGuard.IsObject(value) && index_1.Kind in value && value[index_1.Kind] === kind;
}
/** Returns true if the given value is TLiteral<string> */
function IsLiteralString(value) {
    return IsLiteral(value) && ValueGuard.IsString(value.const);
}
/** Returns true if the given value is TLiteral<number> */
function IsLiteralNumber(value) {
    return IsLiteral(value) && ValueGuard.IsNumber(value.const);
}
/** Returns true if the given value is TLiteral<boolean> */
function IsLiteralBoolean(value) {
    return IsLiteral(value) && ValueGuard.IsBoolean(value.const);
}
/** Returns true if the given value is TLiteral */
function IsLiteral(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Literal') &&
        IsOptionalString(value.$id) && IsLiteralValue(value.const));
}
/** Returns true if the given value is a TLiteralValue */
function IsLiteralValue(value) {
    return ValueGuard.IsBoolean(value) || ValueGuard.IsNumber(value) || ValueGuard.IsString(value);
}
/** Returns true if the given value is a TMappedKey */
function IsMappedKey(value) {
    // prettier-ignore
    return (IsKindOf(value, 'MappedKey') &&
        ValueGuard.IsArray(value.keys) &&
        value.keys.every(key => ValueGuard.IsNumber(key) || ValueGuard.IsString(key)));
}
/** Returns true if the given value is TMappedResult */
function IsMappedResult(value) {
    // prettier-ignore
    return (IsKindOf(value, 'MappedResult') &&
        IsProperties(value.properties));
}
/** Returns true if the given value is TNever */
function IsNever(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Never') &&
        ValueGuard.IsObject(value.not) &&
        Object.getOwnPropertyNames(value.not).length === 0);
}
/** Returns true if the given value is TNot */
function IsNot(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Not') &&
        IsSchema(value.not));
}
/** Returns true if the given value is TNull */
function IsNull(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Null') &&
        value.type === 'null' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TNumber */
function IsNumber(value) {
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
function IsObject(value) {
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
function IsPromise(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Promise') &&
        value.type === 'Promise' &&
        IsOptionalString(value.$id) &&
        IsSchema(value.item));
}
/** Returns true if the given value is TRecord */
function IsRecord(value) {
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
function IsRecursive(value) {
    return ValueGuard.IsObject(value) && index_1.Hint in value && value[index_1.Hint] === 'Recursive';
}
/** Returns true if the given value is TRef */
function IsRef(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Ref') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsString(value.$ref));
}
/** Returns true if the given value is TRegExp */
function IsRegExp(value) {
    // prettier-ignore
    return (IsKindOf(value, 'RegExp') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsString(value.source) &&
        ValueGuard.IsString(value.flags) &&
        IsOptionalNumber(value.maxLength) &&
        IsOptionalNumber(value.minLength));
}
/** Returns true if the given value is TString */
function IsString(value) {
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
function IsSymbol(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Symbol') &&
        value.type === 'symbol' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TTemplateLiteral */
function IsTemplateLiteral(value) {
    // prettier-ignore
    return (IsKindOf(value, 'TemplateLiteral') &&
        value.type === 'string' &&
        ValueGuard.IsString(value.pattern) &&
        value.pattern[0] === '^' &&
        value.pattern[value.pattern.length - 1] === '$');
}
/** Returns true if the given value is TThis */
function IsThis(value) {
    // prettier-ignore
    return (IsKindOf(value, 'This') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsString(value.$ref));
}
/** Returns true of this value is TTransform */
function IsTransform(value) {
    return ValueGuard.IsObject(value) && index_1.TransformKind in value;
}
/** Returns true if the given value is TTuple */
function IsTuple(value) {
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
function IsUndefined(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Undefined') &&
        value.type === 'undefined' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TUnion<Literal<string | number>[]> */
function IsUnionLiteral(value) {
    return IsUnion(value) && value.anyOf.every((schema) => IsLiteralString(schema) || IsLiteralNumber(schema));
}
/** Returns true if the given value is TUnion */
function IsUnion(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Union') &&
        IsOptionalString(value.$id) &&
        ValueGuard.IsObject(value) &&
        ValueGuard.IsArray(value.anyOf) &&
        value.anyOf.every(schema => IsSchema(schema)));
}
/** Returns true if the given value is TUint8Array */
function IsUint8Array(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Uint8Array') &&
        value.type === 'Uint8Array' &&
        IsOptionalString(value.$id) &&
        IsOptionalNumber(value.minByteLength) &&
        IsOptionalNumber(value.maxByteLength));
}
/** Returns true if the given value is TUnknown */
function IsUnknown(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Unknown') &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is a raw TUnsafe */
function IsUnsafe(value) {
    return IsKindOf(value, 'Unsafe');
}
/** Returns true if the given value is TVoid */
function IsVoid(value) {
    // prettier-ignore
    return (IsKindOf(value, 'Void') &&
        value.type === 'void' &&
        IsOptionalString(value.$id));
}
/** Returns true if the given value is TKind */
function IsKind(value) {
    return ValueGuard.IsObject(value) && index_1.Kind in value && ValueGuard.IsString(value[index_1.Kind]) && !KnownTypes.includes(value[index_1.Kind]);
}
/** Returns true if the given value is TSchema */
function IsSchema(value) {
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
