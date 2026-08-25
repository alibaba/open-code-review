import { Kind, Hint, TransformKind } from '../symbols/index';
import { TypeBoxError } from '../error/index';
import { TransformOptions } from '../transform/index';
import type { TAny } from '../any/index';
import type { TArgument } from '../argument/index';
import type { TArray } from '../array/index';
import type { TAsyncIterator } from '../async-iterator/index';
import type { TBoolean } from '../boolean/index';
import type { TComputed } from '../computed/index';
import type { TBigInt } from '../bigint/index';
import type { TConstructor } from '../constructor/index';
import type { TFunction } from '../function/index';
import type { TImport } from '../module/index';
import type { TInteger } from '../integer/index';
import type { TIntersect } from '../intersect/index';
import type { TIterator } from '../iterator/index';
import type { TLiteral, TLiteralValue } from '../literal/index';
import type { TMappedKey, TMappedResult } from '../mapped/index';
import type { TNever } from '../never/index';
import type { TNot } from '../not/index';
import type { TNull } from '../null/index';
import type { TNumber } from '../number/index';
import type { TObject, TProperties } from '../object/index';
import type { TOptional } from '../optional/index';
import type { TPromise } from '../promise/index';
import type { TReadonly } from '../readonly/index';
import type { TRecord } from '../record/index';
import type { TRef } from '../ref/index';
import type { TRegExp } from '../regexp/index';
import type { TSchema } from '../schema/index';
import type { TString } from '../string/index';
import type { TSymbol } from '../symbol/index';
import type { TTemplateLiteral } from '../template-literal/index';
import type { TTuple } from '../tuple/index';
import type { TUint8Array } from '../uint8array/index';
import type { TUndefined } from '../undefined/index';
import type { TUnion } from '../union/index';
import type { TUnknown } from '../unknown/index';
import type { TUnsafe } from '../unsafe/index';
import type { TVoid } from '../void/index';
import type { TDate } from '../date/index';
import type { TThis } from '../recursive/index';
export declare class TypeGuardUnknownTypeError extends TypeBoxError {
}
/** Returns true if this value has a Readonly symbol */
export declare function IsReadonly<T extends TSchema>(value: T): value is TReadonly<T>;
/** Returns true if this value has a Optional symbol */
export declare function IsOptional<T extends TSchema>(value: T): value is TOptional<T>;
/** Returns true if the given value is TAny */
export declare function IsAny(value: unknown): value is TAny;
/** Returns true if the given value is TArgument */
export declare function IsArgument(value: unknown): value is TArgument;
/** Returns true if the given value is TArray */
export declare function IsArray(value: unknown): value is TArray;
/** Returns true if the given value is TAsyncIterator */
export declare function IsAsyncIterator(value: unknown): value is TAsyncIterator;
/** Returns true if the given value is TBigInt */
export declare function IsBigInt(value: unknown): value is TBigInt;
/** Returns true if the given value is TBoolean */
export declare function IsBoolean(value: unknown): value is TBoolean;
/** Returns true if the given value is TComputed */
export declare function IsComputed(value: unknown): value is TComputed;
/** Returns true if the given value is TConstructor */
export declare function IsConstructor(value: unknown): value is TConstructor;
/** Returns true if the given value is TDate */
export declare function IsDate(value: unknown): value is TDate;
/** Returns true if the given value is TFunction */
export declare function IsFunction(value: unknown): value is TFunction;
/** Returns true if the given value is TImport */
export declare function IsImport(value: unknown): value is TImport;
/** Returns true if the given value is TInteger */
export declare function IsInteger(value: unknown): value is TInteger;
/** Returns true if the given schema is TProperties */
export declare function IsProperties(value: unknown): value is TProperties;
/** Returns true if the given value is TIntersect */
export declare function IsIntersect(value: unknown): value is TIntersect;
/** Returns true if the given value is TIterator */
export declare function IsIterator(value: unknown): value is TIterator;
/** Returns true if the given value is a TKind with the given name. */
export declare function IsKindOf<T extends string>(value: unknown, kind: T): value is Record<PropertyKey, unknown> & {
    [Kind]: T;
};
/** Returns true if the given value is TLiteral<string> */
export declare function IsLiteralString(value: unknown): value is TLiteral<string>;
/** Returns true if the given value is TLiteral<number> */
export declare function IsLiteralNumber(value: unknown): value is TLiteral<number>;
/** Returns true if the given value is TLiteral<boolean> */
export declare function IsLiteralBoolean(value: unknown): value is TLiteral<boolean>;
/** Returns true if the given value is TLiteral */
export declare function IsLiteral(value: unknown): value is TLiteral;
/** Returns true if the given value is a TLiteralValue */
export declare function IsLiteralValue(value: unknown): value is TLiteralValue;
/** Returns true if the given value is a TMappedKey */
export declare function IsMappedKey(value: unknown): value is TMappedKey;
/** Returns true if the given value is TMappedResult */
export declare function IsMappedResult(value: unknown): value is TMappedResult;
/** Returns true if the given value is TNever */
export declare function IsNever(value: unknown): value is TNever;
/** Returns true if the given value is TNot */
export declare function IsNot(value: unknown): value is TNot;
/** Returns true if the given value is TNull */
export declare function IsNull(value: unknown): value is TNull;
/** Returns true if the given value is TNumber */
export declare function IsNumber(value: unknown): value is TNumber;
/** Returns true if the given value is TObject */
export declare function IsObject(value: unknown): value is TObject;
/** Returns true if the given value is TPromise */
export declare function IsPromise(value: unknown): value is TPromise;
/** Returns true if the given value is TRecord */
export declare function IsRecord(value: unknown): value is TRecord;
/** Returns true if this value is TRecursive */
export declare function IsRecursive(value: unknown): value is {
    [Hint]: 'Recursive';
};
/** Returns true if the given value is TRef */
export declare function IsRef(value: unknown): value is TRef;
/** Returns true if the given value is TRegExp */
export declare function IsRegExp(value: unknown): value is TRegExp;
/** Returns true if the given value is TString */
export declare function IsString(value: unknown): value is TString;
/** Returns true if the given value is TSymbol */
export declare function IsSymbol(value: unknown): value is TSymbol;
/** Returns true if the given value is TTemplateLiteral */
export declare function IsTemplateLiteral(value: unknown): value is TTemplateLiteral;
/** Returns true if the given value is TThis */
export declare function IsThis(value: unknown): value is TThis;
/** Returns true of this value is TTransform */
export declare function IsTransform(value: unknown): value is {
    [TransformKind]: TransformOptions;
};
/** Returns true if the given value is TTuple */
export declare function IsTuple(value: unknown): value is TTuple;
/** Returns true if the given value is TUndefined */
export declare function IsUndefined(value: unknown): value is TUndefined;
/** Returns true if the given value is TUnion<Literal<string | number>[]> */
export declare function IsUnionLiteral(value: unknown): value is TUnion<TLiteral[]>;
/** Returns true if the given value is TUnion */
export declare function IsUnion(value: unknown): value is TUnion;
/** Returns true if the given value is TUint8Array */
export declare function IsUint8Array(value: unknown): value is TUint8Array;
/** Returns true if the given value is TUnknown */
export declare function IsUnknown(value: unknown): value is TUnknown;
/** Returns true if the given value is a raw TUnsafe */
export declare function IsUnsafe(value: unknown): value is TUnsafe<unknown>;
/** Returns true if the given value is TVoid */
export declare function IsVoid(value: unknown): value is TVoid;
/** Returns true if the given value is TKind */
export declare function IsKind(value: unknown): value is Record<PropertyKey, unknown> & {
    [Kind]: string;
};
/** Returns true if the given value is TSchema */
export declare function IsSchema(value: unknown): value is TSchema;
