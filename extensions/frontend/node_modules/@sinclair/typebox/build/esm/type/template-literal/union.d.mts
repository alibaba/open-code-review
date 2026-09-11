import type { Static } from '../static/index.mjs';
import type { TTemplateLiteral } from './template-literal.mjs';
import type { UnionToTuple } from '../helpers/index.mjs';
import { type TUnionEvaluated } from '../union/index.mjs';
import { type TLiteral } from '../literal/index.mjs';
export type TTemplateLiteralToUnionLiteralArray<T extends string[], Acc extends TLiteral[] = []> = (T extends [infer L extends string, ...infer R extends string[]] ? TTemplateLiteralToUnionLiteralArray<R, [...Acc, TLiteral<L>]> : Acc);
export type TTemplateLiteralToUnion<T extends TTemplateLiteral, U extends string[] = UnionToTuple<Static<T>>> = TUnionEvaluated<TTemplateLiteralToUnionLiteralArray<U>>;
/** Returns a Union from the given TemplateLiteral */
export declare function TemplateLiteralToUnion<T extends TTemplateLiteral>(schema: TTemplateLiteral): TTemplateLiteralToUnion<T>;
