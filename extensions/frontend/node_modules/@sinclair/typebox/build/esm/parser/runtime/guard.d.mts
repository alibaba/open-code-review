import { IArray, IConst, IContext, IIdent, INumber, IOptional, IRef, IString, ITuple, IUnion } from './types.mjs';
/** Returns true if the value is a Array Parser */
export declare function IsArray(value: unknown): value is IArray;
/** Returns true if the value is a Const Parser */
export declare function IsConst(value: unknown): value is IConst;
/** Returns true if the value is a Context Parser */
export declare function IsContext(value: unknown): value is IContext;
/** Returns true if the value is a Ident Parser */
export declare function IsIdent(value: unknown): value is IIdent;
/** Returns true if the value is a Number Parser */
export declare function IsNumber(value: unknown): value is INumber;
/** Returns true if the value is a Optional Parser */
export declare function IsOptional(value: unknown): value is IOptional;
/** Returns true if the value is a Ref Parser */
export declare function IsRef(value: unknown): value is IRef;
/** Returns true if the value is a String Parser */
export declare function IsString(value: unknown): value is IString;
/** Returns true if the value is a Tuple Parser */
export declare function IsTuple(value: unknown): value is ITuple;
/** Returns true if the value is a Union Parser */
export declare function IsUnion(value: unknown): value is IUnion;
/** Returns true if the value is a Parser */
export declare function IsParser(value: unknown): value is IArray<unknown> | IConst<unknown> | IContext<unknown> | IIdent<unknown> | INumber<unknown> | IOptional<unknown> | IRef<unknown> | IString<unknown> | ITuple<unknown> | IUnion<unknown>;
