import { type TTemplateLiteralGenerate, type TTemplateLiteral } from '../template-literal/index';
import type { TLiteral, TLiteralValue } from '../literal/index';
import type { TInteger } from '../integer/index';
import type { TNumber } from '../number/index';
import type { TSchema } from '../schema/index';
import type { TUnion } from '../union/index';
type TFromTemplateLiteral<TemplateLiteral extends TTemplateLiteral, Keys extends string[] = TTemplateLiteralGenerate<TemplateLiteral>> = (Keys);
type TFromUnion<Types extends TSchema[], Result extends string[] = []> = (Types extends [infer Left extends TSchema, ...infer Right extends TSchema[]] ? TFromUnion<Right, [...Result, ...TIndexPropertyKeys<Left>]> : Result);
type TFromLiteral<LiteralValue extends TLiteralValue> = (LiteralValue extends PropertyKey ? [`${LiteralValue}`] : []);
export type TIndexPropertyKeys<Type extends TSchema> = (Type extends TTemplateLiteral ? TFromTemplateLiteral<Type> : Type extends TUnion<infer Types extends TSchema[]> ? TFromUnion<Types> : Type extends TLiteral<infer Value extends TLiteralValue> ? TFromLiteral<Value> : Type extends TNumber ? ['[number]'] : Type extends TInteger ? ['[number]'] : [
]);
/** Returns a tuple of PropertyKeys derived from the given TSchema */
export declare function IndexPropertyKeys<Type extends TSchema>(type: Type): TIndexPropertyKeys<Type>;
export {};
