"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExtendsResult = exports.ExtendsResolverError = void 0;
exports.ExtendsCheck = ExtendsCheck;
const index_1 = require("../any/index");
const index_2 = require("../function/index");
const index_3 = require("../number/index");
const index_4 = require("../string/index");
const index_5 = require("../unknown/index");
const index_6 = require("../template-literal/index");
const index_7 = require("../patterns/index");
const index_8 = require("../symbols/index");
const index_9 = require("../error/index");
const index_10 = require("../guard/index");
class ExtendsResolverError extends index_9.TypeBoxError {
}
exports.ExtendsResolverError = ExtendsResolverError;
var ExtendsResult;
(function (ExtendsResult) {
    ExtendsResult[ExtendsResult["Union"] = 0] = "Union";
    ExtendsResult[ExtendsResult["True"] = 1] = "True";
    ExtendsResult[ExtendsResult["False"] = 2] = "False";
})(ExtendsResult || (exports.ExtendsResult = ExtendsResult = {}));
// ------------------------------------------------------------------
// IntoBooleanResult
// ------------------------------------------------------------------
// prettier-ignore
function IntoBooleanResult(result) {
    return result === ExtendsResult.False ? result : ExtendsResult.True;
}
// ------------------------------------------------------------------
// Throw
// ------------------------------------------------------------------
// prettier-ignore
function Throw(message) {
    throw new ExtendsResolverError(message);
}
// ------------------------------------------------------------------
// StructuralRight
// ------------------------------------------------------------------
// prettier-ignore
function IsStructuralRight(right) {
    return (index_10.TypeGuard.IsNever(right) ||
        index_10.TypeGuard.IsIntersect(right) ||
        index_10.TypeGuard.IsUnion(right) ||
        index_10.TypeGuard.IsUnknown(right) ||
        index_10.TypeGuard.IsAny(right));
}
// prettier-ignore
function StructuralRight(left, right) {
    return (index_10.TypeGuard.IsNever(right) ? FromNeverRight(left, right) :
        index_10.TypeGuard.IsIntersect(right) ? FromIntersectRight(left, right) :
            index_10.TypeGuard.IsUnion(right) ? FromUnionRight(left, right) :
                index_10.TypeGuard.IsUnknown(right) ? FromUnknownRight(left, right) :
                    index_10.TypeGuard.IsAny(right) ? FromAnyRight(left, right) :
                        Throw('StructuralRight'));
}
// ------------------------------------------------------------------
// Any
// ------------------------------------------------------------------
// prettier-ignore
function FromAnyRight(left, right) {
    return ExtendsResult.True;
}
// prettier-ignore
function FromAny(left, right) {
    return (index_10.TypeGuard.IsIntersect(right) ? FromIntersectRight(left, right) :
        (index_10.TypeGuard.IsUnion(right) && right.anyOf.some((schema) => index_10.TypeGuard.IsAny(schema) || index_10.TypeGuard.IsUnknown(schema))) ? ExtendsResult.True :
            index_10.TypeGuard.IsUnion(right) ? ExtendsResult.Union :
                index_10.TypeGuard.IsUnknown(right) ? ExtendsResult.True :
                    index_10.TypeGuard.IsAny(right) ? ExtendsResult.True :
                        ExtendsResult.Union);
}
// ------------------------------------------------------------------
// Array
// ------------------------------------------------------------------
// prettier-ignore
function FromArrayRight(left, right) {
    return (index_10.TypeGuard.IsUnknown(left) ? ExtendsResult.False :
        index_10.TypeGuard.IsAny(left) ? ExtendsResult.Union :
            index_10.TypeGuard.IsNever(left) ? ExtendsResult.True :
                ExtendsResult.False);
}
// prettier-ignore
function FromArray(left, right) {
    return (index_10.TypeGuard.IsObject(right) && IsObjectArrayLike(right) ? ExtendsResult.True :
        IsStructuralRight(right) ? StructuralRight(left, right) :
            !index_10.TypeGuard.IsArray(right) ? ExtendsResult.False :
                IntoBooleanResult(Visit(left.items, right.items)));
}
// ------------------------------------------------------------------
// AsyncIterator
// ------------------------------------------------------------------
// prettier-ignore
function FromAsyncIterator(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        !index_10.TypeGuard.IsAsyncIterator(right) ? ExtendsResult.False :
            IntoBooleanResult(Visit(left.items, right.items)));
}
// ------------------------------------------------------------------
// BigInt
// ------------------------------------------------------------------
// prettier-ignore
function FromBigInt(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsBigInt(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Boolean
// ------------------------------------------------------------------
// prettier-ignore
function FromBooleanRight(left, right) {
    return (index_10.TypeGuard.IsLiteralBoolean(left) ? ExtendsResult.True :
        index_10.TypeGuard.IsBoolean(left) ? ExtendsResult.True :
            ExtendsResult.False);
}
// prettier-ignore
function FromBoolean(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsBoolean(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Constructor
// ------------------------------------------------------------------
// prettier-ignore
function FromConstructor(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            !index_10.TypeGuard.IsConstructor(right) ? ExtendsResult.False :
                left.parameters.length > right.parameters.length ? ExtendsResult.False :
                    (!left.parameters.every((schema, index) => IntoBooleanResult(Visit(right.parameters[index], schema)) === ExtendsResult.True)) ? ExtendsResult.False :
                        IntoBooleanResult(Visit(left.returns, right.returns)));
}
// ------------------------------------------------------------------
// Date
// ------------------------------------------------------------------
// prettier-ignore
function FromDate(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsDate(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Function
// ------------------------------------------------------------------
// prettier-ignore
function FromFunction(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            !index_10.TypeGuard.IsFunction(right) ? ExtendsResult.False :
                left.parameters.length > right.parameters.length ? ExtendsResult.False :
                    (!left.parameters.every((schema, index) => IntoBooleanResult(Visit(right.parameters[index], schema)) === ExtendsResult.True)) ? ExtendsResult.False :
                        IntoBooleanResult(Visit(left.returns, right.returns)));
}
// ------------------------------------------------------------------
// Integer
// ------------------------------------------------------------------
// prettier-ignore
function FromIntegerRight(left, right) {
    return (index_10.TypeGuard.IsLiteral(left) && index_10.ValueGuard.IsNumber(left.const) ? ExtendsResult.True :
        index_10.TypeGuard.IsNumber(left) || index_10.TypeGuard.IsInteger(left) ? ExtendsResult.True :
            ExtendsResult.False);
}
// prettier-ignore
function FromInteger(left, right) {
    return (index_10.TypeGuard.IsInteger(right) || index_10.TypeGuard.IsNumber(right) ? ExtendsResult.True :
        IsStructuralRight(right) ? StructuralRight(left, right) :
            index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
                index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Intersect
// ------------------------------------------------------------------
// prettier-ignore
function FromIntersectRight(left, right) {
    return right.allOf.every((schema) => Visit(left, schema) === ExtendsResult.True)
        ? ExtendsResult.True
        : ExtendsResult.False;
}
// prettier-ignore
function FromIntersect(left, right) {
    return left.allOf.some((schema) => Visit(schema, right) === ExtendsResult.True)
        ? ExtendsResult.True
        : ExtendsResult.False;
}
// ------------------------------------------------------------------
// Iterator
// ------------------------------------------------------------------
// prettier-ignore
function FromIterator(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        !index_10.TypeGuard.IsIterator(right) ? ExtendsResult.False :
            IntoBooleanResult(Visit(left.items, right.items)));
}
// ------------------------------------------------------------------
// Literal
// ------------------------------------------------------------------
// prettier-ignore
function FromLiteral(left, right) {
    return (index_10.TypeGuard.IsLiteral(right) && right.const === left.const ? ExtendsResult.True :
        IsStructuralRight(right) ? StructuralRight(left, right) :
            index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
                index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                    index_10.TypeGuard.IsString(right) ? FromStringRight(left, right) :
                        index_10.TypeGuard.IsNumber(right) ? FromNumberRight(left, right) :
                            index_10.TypeGuard.IsInteger(right) ? FromIntegerRight(left, right) :
                                index_10.TypeGuard.IsBoolean(right) ? FromBooleanRight(left, right) :
                                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Never
// ------------------------------------------------------------------
// prettier-ignore
function FromNeverRight(left, right) {
    return ExtendsResult.False;
}
// prettier-ignore
function FromNever(left, right) {
    return ExtendsResult.True;
}
// ------------------------------------------------------------------
// Not
// ------------------------------------------------------------------
// prettier-ignore
function UnwrapTNot(schema) {
    let [current, depth] = [schema, 0];
    while (true) {
        if (!index_10.TypeGuard.IsNot(current))
            break;
        current = current.not;
        depth += 1;
    }
    return depth % 2 === 0 ? current : (0, index_5.Unknown)();
}
// prettier-ignore
function FromNot(left, right) {
    // TypeScript has no concept of negated types, and attempts to correctly check the negated
    // type at runtime would put TypeBox at odds with TypeScripts ability to statically infer
    // the type. Instead we unwrap to either unknown or T and continue evaluating.
    // prettier-ignore
    return (index_10.TypeGuard.IsNot(left) ? Visit(UnwrapTNot(left), right) :
        index_10.TypeGuard.IsNot(right) ? Visit(left, UnwrapTNot(right)) :
            Throw('Invalid fallthrough for Not'));
}
// ------------------------------------------------------------------
// Null
// ------------------------------------------------------------------
// prettier-ignore
function FromNull(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsNull(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Number
// ------------------------------------------------------------------
// prettier-ignore
function FromNumberRight(left, right) {
    return (index_10.TypeGuard.IsLiteralNumber(left) ? ExtendsResult.True :
        index_10.TypeGuard.IsNumber(left) || index_10.TypeGuard.IsInteger(left) ? ExtendsResult.True :
            ExtendsResult.False);
}
// prettier-ignore
function FromNumber(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsInteger(right) || index_10.TypeGuard.IsNumber(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Object
// ------------------------------------------------------------------
// prettier-ignore
function IsObjectPropertyCount(schema, count) {
    return Object.getOwnPropertyNames(schema.properties).length === count;
}
// prettier-ignore
function IsObjectStringLike(schema) {
    return IsObjectArrayLike(schema);
}
// prettier-ignore
function IsObjectSymbolLike(schema) {
    return IsObjectPropertyCount(schema, 0) || (IsObjectPropertyCount(schema, 1) && 'description' in schema.properties && index_10.TypeGuard.IsUnion(schema.properties.description) && schema.properties.description.anyOf.length === 2 && ((index_10.TypeGuard.IsString(schema.properties.description.anyOf[0]) &&
        index_10.TypeGuard.IsUndefined(schema.properties.description.anyOf[1])) || (index_10.TypeGuard.IsString(schema.properties.description.anyOf[1]) &&
        index_10.TypeGuard.IsUndefined(schema.properties.description.anyOf[0]))));
}
// prettier-ignore
function IsObjectNumberLike(schema) {
    return IsObjectPropertyCount(schema, 0);
}
// prettier-ignore
function IsObjectBooleanLike(schema) {
    return IsObjectPropertyCount(schema, 0);
}
// prettier-ignore
function IsObjectBigIntLike(schema) {
    return IsObjectPropertyCount(schema, 0);
}
// prettier-ignore
function IsObjectDateLike(schema) {
    return IsObjectPropertyCount(schema, 0);
}
// prettier-ignore
function IsObjectUint8ArrayLike(schema) {
    return IsObjectArrayLike(schema);
}
// prettier-ignore
function IsObjectFunctionLike(schema) {
    const length = (0, index_3.Number)();
    return IsObjectPropertyCount(schema, 0) || (IsObjectPropertyCount(schema, 1) && 'length' in schema.properties && IntoBooleanResult(Visit(schema.properties['length'], length)) === ExtendsResult.True);
}
// prettier-ignore
function IsObjectConstructorLike(schema) {
    return IsObjectPropertyCount(schema, 0);
}
// prettier-ignore
function IsObjectArrayLike(schema) {
    const length = (0, index_3.Number)();
    return IsObjectPropertyCount(schema, 0) || (IsObjectPropertyCount(schema, 1) && 'length' in schema.properties && IntoBooleanResult(Visit(schema.properties['length'], length)) === ExtendsResult.True);
}
// prettier-ignore
function IsObjectPromiseLike(schema) {
    const then = (0, index_2.Function)([(0, index_1.Any)()], (0, index_1.Any)());
    return IsObjectPropertyCount(schema, 0) || (IsObjectPropertyCount(schema, 1) && 'then' in schema.properties && IntoBooleanResult(Visit(schema.properties['then'], then)) === ExtendsResult.True);
}
// ------------------------------------------------------------------
// Property
// ------------------------------------------------------------------
// prettier-ignore
function Property(left, right) {
    return (Visit(left, right) === ExtendsResult.False ? ExtendsResult.False :
        index_10.TypeGuard.IsOptional(left) && !index_10.TypeGuard.IsOptional(right) ? ExtendsResult.False :
            ExtendsResult.True);
}
// prettier-ignore
function FromObjectRight(left, right) {
    return (index_10.TypeGuard.IsUnknown(left) ? ExtendsResult.False :
        index_10.TypeGuard.IsAny(left) ? ExtendsResult.Union : (index_10.TypeGuard.IsNever(left) ||
            (index_10.TypeGuard.IsLiteralString(left) && IsObjectStringLike(right)) ||
            (index_10.TypeGuard.IsLiteralNumber(left) && IsObjectNumberLike(right)) ||
            (index_10.TypeGuard.IsLiteralBoolean(left) && IsObjectBooleanLike(right)) ||
            (index_10.TypeGuard.IsSymbol(left) && IsObjectSymbolLike(right)) ||
            (index_10.TypeGuard.IsBigInt(left) && IsObjectBigIntLike(right)) ||
            (index_10.TypeGuard.IsString(left) && IsObjectStringLike(right)) ||
            (index_10.TypeGuard.IsSymbol(left) && IsObjectSymbolLike(right)) ||
            (index_10.TypeGuard.IsNumber(left) && IsObjectNumberLike(right)) ||
            (index_10.TypeGuard.IsInteger(left) && IsObjectNumberLike(right)) ||
            (index_10.TypeGuard.IsBoolean(left) && IsObjectBooleanLike(right)) ||
            (index_10.TypeGuard.IsUint8Array(left) && IsObjectUint8ArrayLike(right)) ||
            (index_10.TypeGuard.IsDate(left) && IsObjectDateLike(right)) ||
            (index_10.TypeGuard.IsConstructor(left) && IsObjectConstructorLike(right)) ||
            (index_10.TypeGuard.IsFunction(left) && IsObjectFunctionLike(right))) ? ExtendsResult.True :
            (index_10.TypeGuard.IsRecord(left) && index_10.TypeGuard.IsString(RecordKey(left))) ? (() => {
                // When expressing a Record with literal key values, the Record is converted into a Object with
                // the Hint assigned as `Record`. This is used to invert the extends logic.
                return right[index_8.Hint] === 'Record' ? ExtendsResult.True : ExtendsResult.False;
            })() :
                (index_10.TypeGuard.IsRecord(left) && index_10.TypeGuard.IsNumber(RecordKey(left))) ? (() => {
                    return IsObjectPropertyCount(right, 0) ? ExtendsResult.True : ExtendsResult.False;
                })() :
                    ExtendsResult.False);
}
// prettier-ignore
function FromObject(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
            !index_10.TypeGuard.IsObject(right) ? ExtendsResult.False :
                (() => {
                    for (const key of Object.getOwnPropertyNames(right.properties)) {
                        if (!(key in left.properties) && !index_10.TypeGuard.IsOptional(right.properties[key])) {
                            return ExtendsResult.False;
                        }
                        if (index_10.TypeGuard.IsOptional(right.properties[key])) {
                            return ExtendsResult.True;
                        }
                        if (Property(left.properties[key], right.properties[key]) === ExtendsResult.False) {
                            return ExtendsResult.False;
                        }
                    }
                    return ExtendsResult.True;
                })());
}
// ------------------------------------------------------------------
// Promise
// ------------------------------------------------------------------
// prettier-ignore
function FromPromise(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) && IsObjectPromiseLike(right) ? ExtendsResult.True :
            !index_10.TypeGuard.IsPromise(right) ? ExtendsResult.False :
                IntoBooleanResult(Visit(left.item, right.item)));
}
// ------------------------------------------------------------------
// Record
// ------------------------------------------------------------------
// prettier-ignore
function RecordKey(schema) {
    return (index_7.PatternNumberExact in schema.patternProperties ? (0, index_3.Number)() :
        index_7.PatternStringExact in schema.patternProperties ? (0, index_4.String)() :
            Throw('Unknown record key pattern'));
}
// prettier-ignore
function RecordValue(schema) {
    return (index_7.PatternNumberExact in schema.patternProperties ? schema.patternProperties[index_7.PatternNumberExact] :
        index_7.PatternStringExact in schema.patternProperties ? schema.patternProperties[index_7.PatternStringExact] :
            Throw('Unable to get record value schema'));
}
// prettier-ignore
function FromRecordRight(left, right) {
    const [Key, Value] = [RecordKey(right), RecordValue(right)];
    return ((index_10.TypeGuard.IsLiteralString(left) && index_10.TypeGuard.IsNumber(Key) && IntoBooleanResult(Visit(left, Value)) === ExtendsResult.True) ? ExtendsResult.True :
        index_10.TypeGuard.IsUint8Array(left) && index_10.TypeGuard.IsNumber(Key) ? Visit(left, Value) :
            index_10.TypeGuard.IsString(left) && index_10.TypeGuard.IsNumber(Key) ? Visit(left, Value) :
                index_10.TypeGuard.IsArray(left) && index_10.TypeGuard.IsNumber(Key) ? Visit(left, Value) :
                    index_10.TypeGuard.IsObject(left) ? (() => {
                        for (const key of Object.getOwnPropertyNames(left.properties)) {
                            if (Property(Value, left.properties[key]) === ExtendsResult.False) {
                                return ExtendsResult.False;
                            }
                        }
                        return ExtendsResult.True;
                    })() :
                        ExtendsResult.False);
}
// prettier-ignore
function FromRecord(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            !index_10.TypeGuard.IsRecord(right) ? ExtendsResult.False :
                Visit(RecordValue(left), RecordValue(right)));
}
// ------------------------------------------------------------------
// RegExp
// ------------------------------------------------------------------
// prettier-ignore
function FromRegExp(left, right) {
    // Note: RegExp types evaluate as strings, not RegExp objects.
    // Here we remap either into string and continue evaluating.
    const L = index_10.TypeGuard.IsRegExp(left) ? (0, index_4.String)() : left;
    const R = index_10.TypeGuard.IsRegExp(right) ? (0, index_4.String)() : right;
    return Visit(L, R);
}
// ------------------------------------------------------------------
// String
// ------------------------------------------------------------------
// prettier-ignore
function FromStringRight(left, right) {
    return (index_10.TypeGuard.IsLiteral(left) && index_10.ValueGuard.IsString(left.const) ? ExtendsResult.True :
        index_10.TypeGuard.IsString(left) ? ExtendsResult.True :
            ExtendsResult.False);
}
// prettier-ignore
function FromString(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsString(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Symbol
// ------------------------------------------------------------------
// prettier-ignore
function FromSymbol(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsSymbol(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// TemplateLiteral
// ------------------------------------------------------------------
// prettier-ignore
function FromTemplateLiteral(left, right) {
    // TemplateLiteral types are resolved to either unions for finite expressions or string
    // for infinite expressions. Here we call to TemplateLiteralResolver to resolve for
    // either type and continue evaluating.
    return (index_10.TypeGuard.IsTemplateLiteral(left) ? Visit((0, index_6.TemplateLiteralToUnion)(left), right) :
        index_10.TypeGuard.IsTemplateLiteral(right) ? Visit(left, (0, index_6.TemplateLiteralToUnion)(right)) :
            Throw('Invalid fallthrough for TemplateLiteral'));
}
// ------------------------------------------------------------------
// Tuple
// ------------------------------------------------------------------
// prettier-ignore
function IsArrayOfTuple(left, right) {
    return (index_10.TypeGuard.IsArray(right) &&
        left.items !== undefined &&
        left.items.every((schema) => Visit(schema, right.items) === ExtendsResult.True));
}
// prettier-ignore
function FromTupleRight(left, right) {
    return (index_10.TypeGuard.IsNever(left) ? ExtendsResult.True :
        index_10.TypeGuard.IsUnknown(left) ? ExtendsResult.False :
            index_10.TypeGuard.IsAny(left) ? ExtendsResult.Union :
                ExtendsResult.False);
}
// prettier-ignore
function FromTuple(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) && IsObjectArrayLike(right) ? ExtendsResult.True :
            index_10.TypeGuard.IsArray(right) && IsArrayOfTuple(left, right) ? ExtendsResult.True :
                !index_10.TypeGuard.IsTuple(right) ? ExtendsResult.False :
                    (index_10.ValueGuard.IsUndefined(left.items) && !index_10.ValueGuard.IsUndefined(right.items)) || (!index_10.ValueGuard.IsUndefined(left.items) && index_10.ValueGuard.IsUndefined(right.items)) ? ExtendsResult.False :
                        (index_10.ValueGuard.IsUndefined(left.items) && !index_10.ValueGuard.IsUndefined(right.items)) ? ExtendsResult.True :
                            left.items.every((schema, index) => Visit(schema, right.items[index]) === ExtendsResult.True) ? ExtendsResult.True :
                                ExtendsResult.False);
}
// ------------------------------------------------------------------
// Uint8Array
// ------------------------------------------------------------------
// prettier-ignore
function FromUint8Array(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsUint8Array(right) ? ExtendsResult.True :
                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Undefined
// ------------------------------------------------------------------
// prettier-ignore
function FromUndefined(left, right) {
    return (IsStructuralRight(right) ? StructuralRight(left, right) :
        index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
            index_10.TypeGuard.IsRecord(right) ? FromRecordRight(left, right) :
                index_10.TypeGuard.IsVoid(right) ? FromVoidRight(left, right) :
                    index_10.TypeGuard.IsUndefined(right) ? ExtendsResult.True :
                        ExtendsResult.False);
}
// ------------------------------------------------------------------
// Union
// ------------------------------------------------------------------
// prettier-ignore
function FromUnionRight(left, right) {
    return right.anyOf.some((schema) => Visit(left, schema) === ExtendsResult.True)
        ? ExtendsResult.True
        : ExtendsResult.False;
}
// prettier-ignore
function FromUnion(left, right) {
    return left.anyOf.every((schema) => Visit(schema, right) === ExtendsResult.True)
        ? ExtendsResult.True
        : ExtendsResult.False;
}
// ------------------------------------------------------------------
// Unknown
// ------------------------------------------------------------------
// prettier-ignore
function FromUnknownRight(left, right) {
    return ExtendsResult.True;
}
// prettier-ignore
function FromUnknown(left, right) {
    return (index_10.TypeGuard.IsNever(right) ? FromNeverRight(left, right) :
        index_10.TypeGuard.IsIntersect(right) ? FromIntersectRight(left, right) :
            index_10.TypeGuard.IsUnion(right) ? FromUnionRight(left, right) :
                index_10.TypeGuard.IsAny(right) ? FromAnyRight(left, right) :
                    index_10.TypeGuard.IsString(right) ? FromStringRight(left, right) :
                        index_10.TypeGuard.IsNumber(right) ? FromNumberRight(left, right) :
                            index_10.TypeGuard.IsInteger(right) ? FromIntegerRight(left, right) :
                                index_10.TypeGuard.IsBoolean(right) ? FromBooleanRight(left, right) :
                                    index_10.TypeGuard.IsArray(right) ? FromArrayRight(left, right) :
                                        index_10.TypeGuard.IsTuple(right) ? FromTupleRight(left, right) :
                                            index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
                                                index_10.TypeGuard.IsUnknown(right) ? ExtendsResult.True :
                                                    ExtendsResult.False);
}
// ------------------------------------------------------------------
// Void
// ------------------------------------------------------------------
// prettier-ignore
function FromVoidRight(left, right) {
    return (index_10.TypeGuard.IsUndefined(left) ? ExtendsResult.True :
        index_10.TypeGuard.IsUndefined(left) ? ExtendsResult.True :
            ExtendsResult.False);
}
// prettier-ignore
function FromVoid(left, right) {
    return (index_10.TypeGuard.IsIntersect(right) ? FromIntersectRight(left, right) :
        index_10.TypeGuard.IsUnion(right) ? FromUnionRight(left, right) :
            index_10.TypeGuard.IsUnknown(right) ? FromUnknownRight(left, right) :
                index_10.TypeGuard.IsAny(right) ? FromAnyRight(left, right) :
                    index_10.TypeGuard.IsObject(right) ? FromObjectRight(left, right) :
                        index_10.TypeGuard.IsVoid(right) ? ExtendsResult.True :
                            ExtendsResult.False);
}
// prettier-ignore
function Visit(left, right) {
    return (
    // resolvable
    (index_10.TypeGuard.IsTemplateLiteral(left) || index_10.TypeGuard.IsTemplateLiteral(right)) ? FromTemplateLiteral(left, right) :
        (index_10.TypeGuard.IsRegExp(left) || index_10.TypeGuard.IsRegExp(right)) ? FromRegExp(left, right) :
            (index_10.TypeGuard.IsNot(left) || index_10.TypeGuard.IsNot(right)) ? FromNot(left, right) :
                // standard
                index_10.TypeGuard.IsAny(left) ? FromAny(left, right) :
                    index_10.TypeGuard.IsArray(left) ? FromArray(left, right) :
                        index_10.TypeGuard.IsBigInt(left) ? FromBigInt(left, right) :
                            index_10.TypeGuard.IsBoolean(left) ? FromBoolean(left, right) :
                                index_10.TypeGuard.IsAsyncIterator(left) ? FromAsyncIterator(left, right) :
                                    index_10.TypeGuard.IsConstructor(left) ? FromConstructor(left, right) :
                                        index_10.TypeGuard.IsDate(left) ? FromDate(left, right) :
                                            index_10.TypeGuard.IsFunction(left) ? FromFunction(left, right) :
                                                index_10.TypeGuard.IsInteger(left) ? FromInteger(left, right) :
                                                    index_10.TypeGuard.IsIntersect(left) ? FromIntersect(left, right) :
                                                        index_10.TypeGuard.IsIterator(left) ? FromIterator(left, right) :
                                                            index_10.TypeGuard.IsLiteral(left) ? FromLiteral(left, right) :
                                                                index_10.TypeGuard.IsNever(left) ? FromNever(left, right) :
                                                                    index_10.TypeGuard.IsNull(left) ? FromNull(left, right) :
                                                                        index_10.TypeGuard.IsNumber(left) ? FromNumber(left, right) :
                                                                            index_10.TypeGuard.IsObject(left) ? FromObject(left, right) :
                                                                                index_10.TypeGuard.IsRecord(left) ? FromRecord(left, right) :
                                                                                    index_10.TypeGuard.IsString(left) ? FromString(left, right) :
                                                                                        index_10.TypeGuard.IsSymbol(left) ? FromSymbol(left, right) :
                                                                                            index_10.TypeGuard.IsTuple(left) ? FromTuple(left, right) :
                                                                                                index_10.TypeGuard.IsPromise(left) ? FromPromise(left, right) :
                                                                                                    index_10.TypeGuard.IsUint8Array(left) ? FromUint8Array(left, right) :
                                                                                                        index_10.TypeGuard.IsUndefined(left) ? FromUndefined(left, right) :
                                                                                                            index_10.TypeGuard.IsUnion(left) ? FromUnion(left, right) :
                                                                                                                index_10.TypeGuard.IsUnknown(left) ? FromUnknown(left, right) :
                                                                                                                    index_10.TypeGuard.IsVoid(left) ? FromVoid(left, right) :
                                                                                                                        Throw(`Unknown left type operand '${left[index_8.Kind]}'`));
}
function ExtendsCheck(left, right) {
    return Visit(left, right);
}
