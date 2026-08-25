import { Kind, Hint, TransformKind } from '../symbols/index.mjs';
import { TransformOptions } from '../transform/index.mjs';
import type { TAny } from '../any/index.mjs';
import type { TArgument } from '../argument/index.mjs';
import type { TArray } from '../array/index.mjs';
import type { TAsyncIterator } from '../async-iterator/index.mjs';
import type { TBoolean } from '../boolean/index.mjs';
import type { TComputed } from '../computed/index.mjs';
import type { TBigInt } from '../bigint/index.mjs';
import type { TConstructor } from '../constructor/index.mjs';
import type { TFunction } from '../function/index.mjs';
import type { TImport } from '../module/index.mjs';
import type { TInteger } from '../integer/index.mjs';
import type { TIntersect } from '../intersect/index.mjs';
import type { TIterator } from '../iterator/index.mjs';
import type { TLiteral, TLiteralValue } from '../literal/index.mjs';
import type { TMappedKey, TMappedResult } from '../mapped/index.mjs';
import type { TNever } from '../never/index.mjs';
import type { TNot } from '../not/index.mjs';
import type { TNull } from '../null/index.mjs';
import type { TNumber } from '../number/index.mjs';
import type { TObject, TProperties } from '../object/index.mjs';
import type { TOptional } from '../optional/index.mjs';
import type { TPromise } from '../promise/index.mjs';
import type { TReadonly } from '../readonly/index.mjs';
import type { TRecord } from '../record/index.mjs';
import type { TRef } from '../ref/index.mjs';
import type { TRegExp } from '../regexp/index.mjs';
import type { TSchema } from '../schema/index.mjs';
import type { TString } from '../string/index.mjs';
import type { TSymbol } from '../symbol/index.mjs';
import type { TTemplateLiteral } from '../template-literal/index.mjs';
import type { TTuple } from '../tuple/index.mjs';
import type { TUint8Array } from '../uint8array/index.mjs';
import type { TUndefined } from '../undefined/index.mjs';
import type { TUnknown } from '../unknown/index.mjs';
import type { TUnion } from '../union/index.mjs';
import type { TUnsafe } from '../unsafe/index.mjs';
import type { TVoid } from '../void/index.mjs';
import type { TDate } from '../date/index.mjs';
import type { TThis } from '../recursive/index.mjs';
/** `[Kind-Only]` Returns true if this value has a Readonly symbol */
export declare function IsReadonly<T extends TSchema>(value: T): value is TReadonly<T>;
/** `[Kind-Only]` Returns true if this value has a Optional symbol */
export declare function IsOptional<T extends TSchema>(value: T): value is TOptional<T>;
/** `[Kind-Only]` Returns true if the given value is TAny */
export declare function IsAny(value: unknown): value is TAny;
/** `[Kind-Only]` Returns true if the given value is TArgument */
export declare function IsArgument(value: unknown): value is TArgument;
/** `[Kind-Only]` Returns true if the given value is TArray */
export declare function IsArray(value: unknown): value is TArray;
/** `[Kind-Only]` Returns true if the given value is TAsyncIterator */
export declare function IsAsyncIterator(value: unknown): value is TAsyncIterator;
/** `[Kind-Only]` Returns true if the given value is TBigInt */
export declare function IsBigInt(value: unknown): value is TBigInt;
/** `[Kind-Only]` Returns true if the given value is TBoolean */
export declare function IsBoolean(value: unknown): value is TBoolean;
/** `[Kind-Only]` Returns true if the given value is TComputed */
export declare function IsComputed(value: unknown): value is TComputed;
/** `[Kind-Only]` Returns true if the given value is TConstructor */
export declare function IsConstructor(value: unknown): value is TConstructor;
/** `[Kind-Only]` Returns true if the given value is TDate */
export declare function IsDate(value: unknown): value is TDate;
/** `[Kind-Only]` Returns true if the given value is TFunction */
export declare function IsFunction(value: unknown): value is TFunction;
/** `[Kind-Only]` Returns true if the given value is TInteger */
export declare function IsImport(value: unknown): value is TImport;
/** `[Kind-Only]` Returns true if the given value is TInteger */
export declare function IsInteger(value: unknown): value is TInteger;
/** `[Kind-Only]` Returns true if the given schema is TProperties */
export declare function IsProperties(value: unknown): value is TProperties;
/** `[Kind-Only]` Returns true if the given value is TIntersect */
export declare function IsIntersect(value: unknown): value is TIntersect;
/** `[Kind-Only]` Returns true if the given value is TIterator */
export declare function IsIterator(value: unknown): value is TIterator;
/** `[Kind-Only]` Returns true if the given value is a TKind with the given name. */
export declare function IsKindOf<T extends string>(value: unknown, kind: T): value is Record<PropertyKey, unknown> & {
    [Kind]: T;
};
/** `[Kind-Only]` Returns true if the given value is TLiteral<string> */
export declare function IsLiteralString(value: unknown): value is TLiteral<string>;
/** `[Kind-Only]` Returns true if the given value is TLiteral<number> */
export declare function IsLiteralNumber(value: unknown): value is TLiteral<number>;
/** `[Kind-Only]` Returns true if the given value is TLiteral<boolean> */
export declare function IsLiteralBoolean(value: unknown): value is TLiteral<boolean>;
/** `[Kind-Only]` Returns true if the given value is TLiteralValue */
export declare function IsLiteralValue(value: unknown): value is TLiteralValue;
/** `[Kind-Only]` Returns true if the given value is TLiteral */
export declare function IsLiteral(value: unknown): value is TLiteral;
/** `[Kind-Only]` Returns true if the given value is a TMappedKey */
export declare function IsMappedKey(value: unknown): value is TMappedKey;
/** `[Kind-Only]` Returns true if the given value is TMappedResult */
export declare function IsMappedResult(value: unknown): value is TMappedResult;
/** `[Kind-Only]` Returns true if the given value is TNever */
export declare function IsNever(value: unknown): value is TNever;
/** `[Kind-Only]` Returns true if the given value is TNot */
export declare function IsNot(value: unknown): value is TNot;
/** `[Kind-Only]` Returns true if the given value is TNull */
export declare function IsNull(value: unknown): value is TNull;
/** `[Kind-Only]` Returns true if the given value is TNumber */
export declare function IsNumber(value: unknown): value is TNumber;
/** `[Kind-Only]` Returns true if the given value is TObject */
export declare function IsObject(value: unknown): value is TObject;
/** `[Kind-Only]` Returns true if the given value is TPromise */
export declare function IsPromise(value: unknown): value is TPromise;
/** `[Kind-Only]` Returns true if the given value is TRecord */
export declare function IsRecord(value: unknown): value is TRecord;
/** `[Kind-Only]` Returns true if this value is TRecursive */
export declare function IsRecursive(value: unknown): value is {
    [Hint]: 'Recursive';
};
/** `[Kind-Only]` Returns true if the given value is TRef */
export declare function IsRef(value: unknown): value is TRef;
/** `[Kind-Only]` Returns true if the given value is TRegExp */
export declare function IsRegExp(value: unknown): value is TRegExp;
/** `[Kind-Only]` Returns true if the given value is TString */
export declare function IsString(value: unknown): value is TString;
/** `[Kind-Only]` Returns true if the given value is TSymbol */
export declare function IsSymbol(value: unknown): value is TSymbol;
/** `[Kind-Only]` Returns true if the given value is TTemplateLiteral */
export declare function IsTemplateLiteral(value: unknown): value is TTemplateLiteral;
/** `[Kind-Only]` Returns true if the given value is TThis */
export declare function IsThis(value: unknown): value is TThis;
/** `[Kind-Only]` Returns true of this value is TTransform */
export declare function IsTransform(value: unknown): value is {
    [TransformKind]: TransformOptions;
};
/** `[Kind-Only]` Returns true if the given value is TTuple */
export declare function IsTuple(value: unknown): value is TTuple;
/** `[Kind-Only]` Returns true if the given value is TUndefined */
export declare function IsUndefined(value: unknown): value is TUndefined;
/** `[Kind-Only]` Returns true if the given value is TUnion */
export declare function IsUnion(value: unknown): value is TUnion;
/** `[Kind-Only]` Returns true if the given value is TUint8Array */
export declare function IsUint8Array(value: unknown): value is TUint8Array;
/** `[Kind-Only]` Returns true if the given value is TUnknown */
export declare function IsUnknown(value: unknown): value is TUnknown;
/** `[Kind-Only]` Returns true if the given value is a raw TUnsafe */
export declare function IsUnsafe(value: unknown): value is TUnsafe<unknown>;
/** `[Kind-Only]` Returns true if the given value is TVoid */
export declare function IsVoid(value: unknown): value is TVoid;
/** `[Kind-Only]` Returns true if the given value is TKind */
export declare function IsKind(value: unknown): value is Record<PropertyKey, unknown> & {
    [Kind]: string;
};
/** `[Kind-Only]` Returns true if the given value is TSchema */
export declare function IsSchema(value: unknown): value is TSchema;
