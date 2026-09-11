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
exports.IsLiteralValue = IsLiteralValue;
exports.IsLiteral = IsLiteral;
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
exports.IsUnion = IsUnion;
exports.IsUint8Array = IsUint8Array;
exports.IsUnknown = IsUnknown;
exports.IsUnsafe = IsUnsafe;
exports.IsVoid = IsVoid;
exports.IsKind = IsKind;
exports.IsSchema = IsSchema;
const ValueGuard = __importStar(require("./value"));
const index_1 = require("../symbols/index");
/** `[Kind-Only]` Returns true if this value has a Readonly symbol */
function IsReadonly(value) {
    return ValueGuard.IsObject(value) && value[index_1.ReadonlyKind] === 'Readonly';
}
/** `[Kind-Only]` Returns true if this value has a Optional symbol */
function IsOptional(value) {
    return ValueGuard.IsObject(value) && value[index_1.OptionalKind] === 'Optional';
}
/** `[Kind-Only]` Returns true if the given value is TAny */
function IsAny(value) {
    return IsKindOf(value, 'Any');
}
/** `[Kind-Only]` Returns true if the given value is TArgument */
function IsArgument(value) {
    return IsKindOf(value, 'Argument');
}
/** `[Kind-Only]` Returns true if the given value is TArray */
function IsArray(value) {
    return IsKindOf(value, 'Array');
}
/** `[Kind-Only]` Returns true if the given value is TAsyncIterator */
function IsAsyncIterator(value) {
    return IsKindOf(value, 'AsyncIterator');
}
/** `[Kind-Only]` Returns true if the given value is TBigInt */
function IsBigInt(value) {
    return IsKindOf(value, 'BigInt');
}
/** `[Kind-Only]` Returns true if the given value is TBoolean */
function IsBoolean(value) {
    return IsKindOf(value, 'Boolean');
}
/** `[Kind-Only]` Returns true if the given value is TComputed */
function IsComputed(value) {
    return IsKindOf(value, 'Computed');
}
/** `[Kind-Only]` Returns true if the given value is TConstructor */
function IsConstructor(value) {
    return IsKindOf(value, 'Constructor');
}
/** `[Kind-Only]` Returns true if the given value is TDate */
function IsDate(value) {
    return IsKindOf(value, 'Date');
}
/** `[Kind-Only]` Returns true if the given value is TFunction */
function IsFunction(value) {
    return IsKindOf(value, 'Function');
}
/** `[Kind-Only]` Returns true if the given value is TInteger */
function IsImport(value) {
    return IsKindOf(value, 'Import');
}
/** `[Kind-Only]` Returns true if the given value is TInteger */
function IsInteger(value) {
    return IsKindOf(value, 'Integer');
}
/** `[Kind-Only]` Returns true if the given schema is TProperties */
function IsProperties(value) {
    return ValueGuard.IsObject(value);
}
/** `[Kind-Only]` Returns true if the given value is TIntersect */
function IsIntersect(value) {
    return IsKindOf(value, 'Intersect');
}
/** `[Kind-Only]` Returns true if the given value is TIterator */
function IsIterator(value) {
    return IsKindOf(value, 'Iterator');
}
/** `[Kind-Only]` Returns true if the given value is a TKind with the given name. */
function IsKindOf(value, kind) {
    return ValueGuard.IsObject(value) && index_1.Kind in value && value[index_1.Kind] === kind;
}
/** `[Kind-Only]` Returns true if the given value is TLiteral<string> */
function IsLiteralString(value) {
    return IsLiteral(value) && ValueGuard.IsString(value.const);
}
/** `[Kind-Only]` Returns true if the given value is TLiteral<number> */
function IsLiteralNumber(value) {
    return IsLiteral(value) && ValueGuard.IsNumber(value.const);
}
/** `[Kind-Only]` Returns true if the given value is TLiteral<boolean> */
function IsLiteralBoolean(value) {
    return IsLiteral(value) && ValueGuard.IsBoolean(value.const);
}
/** `[Kind-Only]` Returns true if the given value is TLiteralValue */
function IsLiteralValue(value) {
    return ValueGuard.IsBoolean(value) || ValueGuard.IsNumber(value) || ValueGuard.IsString(value);
}
/** `[Kind-Only]` Returns true if the given value is TLiteral */
function IsLiteral(value) {
    return IsKindOf(value, 'Literal');
}
/** `[Kind-Only]` Returns true if the given value is a TMappedKey */
function IsMappedKey(value) {
    return IsKindOf(value, 'MappedKey');
}
/** `[Kind-Only]` Returns true if the given value is TMappedResult */
function IsMappedResult(value) {
    return IsKindOf(value, 'MappedResult');
}
/** `[Kind-Only]` Returns true if the given value is TNever */
function IsNever(value) {
    return IsKindOf(value, 'Never');
}
/** `[Kind-Only]` Returns true if the given value is TNot */
function IsNot(value) {
    return IsKindOf(value, 'Not');
}
/** `[Kind-Only]` Returns true if the given value is TNull */
function IsNull(value) {
    return IsKindOf(value, 'Null');
}
/** `[Kind-Only]` Returns true if the given value is TNumber */
function IsNumber(value) {
    return IsKindOf(value, 'Number');
}
/** `[Kind-Only]` Returns true if the given value is TObject */
function IsObject(value) {
    return IsKindOf(value, 'Object');
}
/** `[Kind-Only]` Returns true if the given value is TPromise */
function IsPromise(value) {
    return IsKindOf(value, 'Promise');
}
/** `[Kind-Only]` Returns true if the given value is TRecord */
function IsRecord(value) {
    return IsKindOf(value, 'Record');
}
/** `[Kind-Only]` Returns true if this value is TRecursive */
function IsRecursive(value) {
    return ValueGuard.IsObject(value) && index_1.Hint in value && value[index_1.Hint] === 'Recursive';
}
/** `[Kind-Only]` Returns true if the given value is TRef */
function IsRef(value) {
    return IsKindOf(value, 'Ref');
}
/** `[Kind-Only]` Returns true if the given value is TRegExp */
function IsRegExp(value) {
    return IsKindOf(value, 'RegExp');
}
/** `[Kind-Only]` Returns true if the given value is TString */
function IsString(value) {
    return IsKindOf(value, 'String');
}
/** `[Kind-Only]` Returns true if the given value is TSymbol */
function IsSymbol(value) {
    return IsKindOf(value, 'Symbol');
}
/** `[Kind-Only]` Returns true if the given value is TTemplateLiteral */
function IsTemplateLiteral(value) {
    return IsKindOf(value, 'TemplateLiteral');
}
/** `[Kind-Only]` Returns true if the given value is TThis */
function IsThis(value) {
    return IsKindOf(value, 'This');
}
/** `[Kind-Only]` Returns true of this value is TTransform */
function IsTransform(value) {
    return ValueGuard.IsObject(value) && index_1.TransformKind in value;
}
/** `[Kind-Only]` Returns true if the given value is TTuple */
function IsTuple(value) {
    return IsKindOf(value, 'Tuple');
}
/** `[Kind-Only]` Returns true if the given value is TUndefined */
function IsUndefined(value) {
    return IsKindOf(value, 'Undefined');
}
/** `[Kind-Only]` Returns true if the given value is TUnion */
function IsUnion(value) {
    return IsKindOf(value, 'Union');
}
/** `[Kind-Only]` Returns true if the given value is TUint8Array */
function IsUint8Array(value) {
    return IsKindOf(value, 'Uint8Array');
}
/** `[Kind-Only]` Returns true if the given value is TUnknown */
function IsUnknown(value) {
    return IsKindOf(value, 'Unknown');
}
/** `[Kind-Only]` Returns true if the given value is a raw TUnsafe */
function IsUnsafe(value) {
    return IsKindOf(value, 'Unsafe');
}
/** `[Kind-Only]` Returns true if the given value is TVoid */
function IsVoid(value) {
    return IsKindOf(value, 'Void');
}
/** `[Kind-Only]` Returns true if the given value is TKind */
function IsKind(value) {
    return ValueGuard.IsObject(value) && index_1.Kind in value && ValueGuard.IsString(value[index_1.Kind]);
}
/** `[Kind-Only]` Returns true if the given value is TSchema */
function IsSchema(value) {
    // prettier-ignore
    return (IsAny(value) ||
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
