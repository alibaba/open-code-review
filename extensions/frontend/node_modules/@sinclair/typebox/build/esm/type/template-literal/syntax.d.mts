import type { Assert, Trim } from '../helpers/index.mjs';
import type { TTemplateLiteral, TTemplateLiteralKind } from './index.mjs';
import { type TLiteral } from '../literal/index.mjs';
import { type TBoolean } from '../boolean/index.mjs';
import { type TBigInt } from '../bigint/index.mjs';
import { type TNumber } from '../number/index.mjs';
import { type TString } from '../string/index.mjs';
import { type TUnionEvaluated } from '../union/index.mjs';
declare function FromUnion(syntax: string): IterableIterator<TTemplateLiteralKind>;
declare function FromTerminal(syntax: string): IterableIterator<TTemplateLiteralKind>;
type FromUnionLiteral<T extends string> = T extends `${infer L}|${infer R}` ? [TLiteral<Trim<L>>, ...FromUnionLiteral<R>] : T extends `${infer L}` ? [TLiteral<Trim<L>>] : [
];
type FromUnion<T extends string> = TUnionEvaluated<FromUnionLiteral<T>>;
type FromTerminal<T extends string> = T extends 'boolean' ? TBoolean : T extends 'bigint' ? TBigInt : T extends 'number' ? TNumber : T extends 'string' ? TString : FromUnion<T>;
type FromString<T extends string> = T extends `{${infer L}}${infer R}` ? [FromTerminal<L>, ...FromString<R>] : T extends `${infer L}$\{${infer R1}\}${infer R2}` ? [TLiteral<L>, ...FromString<`{${R1}}`>, ...FromString<R2>] : T extends `${infer L}$\{${infer R1}\}` ? [TLiteral<L>, ...FromString<`{${R1}}`>] : T extends `${infer L}` ? [TLiteral<L>] : [
];
export type TTemplateLiteralSyntax<T extends string> = (TTemplateLiteral<Assert<FromString<T>, TTemplateLiteralKind[]>>);
/** Parses TemplateLiteralSyntax and returns a tuple of TemplateLiteralKinds */
export declare function TemplateLiteralSyntax(syntax: string): TTemplateLiteralKind[];
export {};
