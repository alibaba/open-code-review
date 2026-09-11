import * as ValueGuard from './value.mjs';
import { Kind, Hint, TransformKind, ReadonlyKind, OptionalKind } from '../symbols/index.mjs';
/** `[Kind-Only]` Returns true if this value has a Readonly symbol */
export function IsReadonly(value) {
    return ValueGuard.IsObject(value) && value[ReadonlyKind] === 'Readonly';
}
/** `[Kind-Only]` Returns true if this value has a Optional symbol */
export function IsOptional(value) {
    return ValueGuard.IsObject(value) && value[OptionalKind] === 'Optional';
}
/** `[Kind-Only]` Returns true if the given value is TAny */
export function IsAny(value) {
    return IsKindOf(value, 'Any');
}
/** `[Kind-Only]` Returns true if the given value is TArgument */
export function IsArgument(value) {
    return IsKindOf(value, 'Argument');
}
/** `[Kind-Only]` Returns true if the given value is TArray */
export function IsArray(value) {
    return IsKindOf(value, 'Array');
}
/** `[Kind-Only]` Returns true if the given value is TAsyncIterator */
export function IsAsyncIterator(value) {
    return IsKindOf(value, 'AsyncIterator');
}
/** `[Kind-Only]` Returns true if the given value is TBigInt */
export function IsBigInt(value) {
    return IsKindOf(value, 'BigInt');
}
/** `[Kind-Only]` Returns true if the given value is TBoolean */
export function IsBoolean(value) {
    return IsKindOf(value, 'Boolean');
}
/** `[Kind-Only]` Returns true if the given value is TComputed */
export function IsComputed(value) {
    return IsKindOf(value, 'Computed');
}
/** `[Kind-Only]` Returns true if the given value is TConstructor */
export function IsConstructor(value) {
    return IsKindOf(value, 'Constructor');
}
/** `[Kind-Only]` Returns true if the given value is TDate */
export function IsDate(value) {
    return IsKindOf(value, 'Date');
}
/** `[Kind-Only]` Returns true if the given value is TFunction */
export function IsFunction(value) {
    return IsKindOf(value, 'Function');
}
/** `[Kind-Only]` Returns true if the given value is TInteger */
export function IsImport(value) {
    return IsKindOf(value, 'Import');
}
/** `[Kind-Only]` Returns true if the given value is TInteger */
export function IsInteger(value) {
    return IsKindOf(value, 'Integer');
}
/** `[Kind-Only]` Returns true if the given schema is TProperties */
export function IsProperties(value) {
    return ValueGuard.IsObject(value);
}
/** `[Kind-Only]` Returns true if the given value is TIntersect */
export function IsIntersect(value) {
    return IsKindOf(value, 'Intersect');
}
/** `[Kind-Only]` Returns true if the given value is TIterator */
export function IsIterator(value) {
    return IsKindOf(value, 'Iterator');
}
/** `[Kind-Only]` Returns true if the given value is a TKind with the given name. */
export function IsKindOf(value, kind) {
    return ValueGuard.IsObject(value) && Kind in value && value[Kind] === kind;
}
/** `[Kind-Only]` Returns true if the given value is TLiteral<string> */
export function IsLiteralString(value) {
    return IsLiteral(value) && ValueGuard.IsString(value.const);
}
/** `[Kind-Only]` Returns true if the given value is TLiteral<number> */
export function IsLiteralNumber(value) {
    return IsLiteral(value) && ValueGuard.IsNumber(value.const);
}
/** `[Kind-Only]` Returns true if the given value is TLiteral<boolean> */
export function IsLiteralBoolean(value) {
    return IsLiteral(value) && ValueGuard.IsBoolean(value.const);
}
/** `[Kind-Only]` Returns true if the given value is TLiteralValue */
export function IsLiteralValue(value) {
    return ValueGuard.IsBoolean(value) || ValueGuard.IsNumber(value) || ValueGuard.IsString(value);
}
/** `[Kind-Only]` Returns true if the given value is TLiteral */
export function IsLiteral(value) {
    return IsKindOf(value, 'Literal');
}
/** `[Kind-Only]` Returns true if the given value is a TMappedKey */
export function IsMappedKey(value) {
    return IsKindOf(value, 'MappedKey');
}
/** `[Kind-Only]` Returns true if the given value is TMappedResult */
export function IsMappedResult(value) {
    return IsKindOf(value, 'MappedResult');
}
/** `[Kind-Only]` Returns true if the given value is TNever */
export function IsNever(value) {
    return IsKindOf(value, 'Never');
}
/** `[Kind-Only]` Returns true if the given value is TNot */
export function IsNot(value) {
    return IsKindOf(value, 'Not');
}
/** `[Kind-Only]` Returns true if the given value is TNull */
export function IsNull(value) {
    return IsKindOf(value, 'Null');
}
/** `[Kind-Only]` Returns true if the given value is TNumber */
export function IsNumber(value) {
    return IsKindOf(value, 'Number');
}
/** `[Kind-Only]` Returns true if the given value is TObject */
export function IsObject(value) {
    return IsKindOf(value, 'Object');
}
/** `[Kind-Only]` Returns true if the given value is TPromise */
export function IsPromise(value) {
    return IsKindOf(value, 'Promise');
}
/** `[Kind-Only]` Returns true if the given value is TRecord */
export function IsRecord(value) {
    return IsKindOf(value, 'Record');
}
/** `[Kind-Only]` Returns true if this value is TRecursive */
export function IsRecursive(value) {
    return ValueGuard.IsObject(value) && Hint in value && value[Hint] === 'Recursive';
}
/** `[Kind-Only]` Returns true if the given value is TRef */
export function IsRef(value) {
    return IsKindOf(value, 'Ref');
}
/** `[Kind-Only]` Returns true if the given value is TRegExp */
export function IsRegExp(value) {
    return IsKindOf(value, 'RegExp');
}
/** `[Kind-Only]` Returns true if the given value is TString */
export function IsString(value) {
    return IsKindOf(value, 'String');
}
/** `[Kind-Only]` Returns true if the given value is TSymbol */
export function IsSymbol(value) {
    return IsKindOf(value, 'Symbol');
}
/** `[Kind-Only]` Returns true if the given value is TTemplateLiteral */
export function IsTemplateLiteral(value) {
    return IsKindOf(value, 'TemplateLiteral');
}
/** `[Kind-Only]` Returns true if the given value is TThis */
export function IsThis(value) {
    return IsKindOf(value, 'This');
}
/** `[Kind-Only]` Returns true of this value is TTransform */
export function IsTransform(value) {
    return ValueGuard.IsObject(value) && TransformKind in value;
}
/** `[Kind-Only]` Returns true if the given value is TTuple */
export function IsTuple(value) {
    return IsKindOf(value, 'Tuple');
}
/** `[Kind-Only]` Returns true if the given value is TUndefined */
export function IsUndefined(value) {
    return IsKindOf(value, 'Undefined');
}
/** `[Kind-Only]` Returns true if the given value is TUnion */
export function IsUnion(value) {
    return IsKindOf(value, 'Union');
}
/** `[Kind-Only]` Returns true if the given value is TUint8Array */
export function IsUint8Array(value) {
    return IsKindOf(value, 'Uint8Array');
}
/** `[Kind-Only]` Returns true if the given value is TUnknown */
export function IsUnknown(value) {
    return IsKindOf(value, 'Unknown');
}
/** `[Kind-Only]` Returns true if the given value is a raw TUnsafe */
export function IsUnsafe(value) {
    return IsKindOf(value, 'Unsafe');
}
/** `[Kind-Only]` Returns true if the given value is TVoid */
export function IsVoid(value) {
    return IsKindOf(value, 'Void');
}
/** `[Kind-Only]` Returns true if the given value is TKind */
export function IsKind(value) {
    return ValueGuard.IsObject(value) && Kind in value && ValueGuard.IsString(value[Kind]);
}
/** `[Kind-Only]` Returns true if the given value is TSchema */
export function IsSchema(value) {
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
