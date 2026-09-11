import { TIsTemplateLiteralFinite } from './finite.mjs';
import { TypeBoxError } from '../error/index.mjs';
import type { Assert } from '../helpers/index.mjs';
import type { TBoolean } from '../boolean/index.mjs';
import type { TTemplateLiteral, TTemplateLiteralKind } from './index.mjs';
import type { TLiteral, TLiteralValue } from '../literal/index.mjs';
import type { Expression } from './parse.mjs';
import type { TUnion } from '../union/index.mjs';
export declare class TemplateLiteralGenerateError extends TypeBoxError {
}
type TStringReduceUnary<L extends string, R extends string[], Acc extends string[] = []> = R extends [infer A extends string, ...infer B extends string[]] ? TStringReduceUnary<L, B, [...Acc, `${L}${A}`]> : Acc;
type TStringReduceBinary<L extends string[], R extends string[], Acc extends string[] = []> = L extends [infer A extends string, ...infer B extends string[]] ? TStringReduceBinary<B, R, [...Acc, ...TStringReduceUnary<A, R>]> : Acc;
type TStringReduceMany<T extends string[][]> = T extends [infer L extends string[], infer R extends string[], ...infer Rest extends string[][]] ? TStringReduceMany<[TStringReduceBinary<L, R>, ...Rest]> : T;
type TStringReduce<T extends string[][], O = TStringReduceMany<T>> = 0 extends keyof O ? Assert<O[0], string[]> : [];
type TFromTemplateLiteralUnionKinds<T extends TTemplateLiteralKind[]> = T extends [infer L extends TLiteral, ...infer R extends TLiteral[]] ? [`${L['const']}`, ...TFromTemplateLiteralUnionKinds<R>] : [];
type TFromTemplateLiteralKinds<T extends TTemplateLiteralKind[], Acc extends TLiteralValue[][] = []> = T extends [infer L extends TTemplateLiteralKind, ...infer R extends TTemplateLiteralKind[]] ? (L extends TTemplateLiteral<infer S extends TTemplateLiteralKind[]> ? TFromTemplateLiteralKinds<[...S, ...R], Acc> : L extends TLiteral<infer S extends TLiteralValue> ? TFromTemplateLiteralKinds<R, [...Acc, [S]]> : L extends TUnion<infer S extends TTemplateLiteralKind[]> ? TFromTemplateLiteralKinds<R, [...Acc, TFromTemplateLiteralUnionKinds<S>]> : L extends TBoolean ? TFromTemplateLiteralKinds<R, [...Acc, ['true', 'false']]> : Acc) : Acc;
export declare function TemplateLiteralExpressionGenerate(expression: Expression): IterableIterator<string>;
export type TTemplateLiteralGenerate<T extends TTemplateLiteral, F = TIsTemplateLiteralFinite<T>> = F extends true ? (T extends TTemplateLiteral<infer S extends TTemplateLiteralKind[]> ? TFromTemplateLiteralKinds<S> extends infer R extends string[][] ? TStringReduce<R> : [] : []) : [];
/** Generates a tuple of strings from the given TemplateLiteral. Returns an empty tuple if infinite. */
export declare function TemplateLiteralGenerate<T extends TTemplateLiteral>(schema: T): TTemplateLiteralGenerate<T>;
export {};
