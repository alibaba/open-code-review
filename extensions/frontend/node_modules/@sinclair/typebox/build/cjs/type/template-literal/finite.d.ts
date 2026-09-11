import { TypeBoxError } from '../error/index';
import type { TTemplateLiteral, TTemplateLiteralKind } from './index';
import type { TUnion } from '../union/index';
import type { TString } from '../string/index';
import type { TBoolean } from '../boolean/index';
import type { TNumber } from '../number/index';
import type { TInteger } from '../integer/index';
import type { TBigInt } from '../bigint/index';
import type { TLiteral } from '../literal/index';
import type { Expression } from './parse';
export declare class TemplateLiteralFiniteError extends TypeBoxError {
}
type TFromTemplateLiteralKind<T> = T extends TTemplateLiteral<infer U extends TTemplateLiteralKind[]> ? TFromTemplateLiteralKinds<U> : T extends TUnion<infer U extends TTemplateLiteralKind[]> ? TFromTemplateLiteralKinds<U> : T extends TString ? false : T extends TNumber ? false : T extends TInteger ? false : T extends TBigInt ? false : T extends TBoolean ? true : T extends TLiteral ? true : false;
type TFromTemplateLiteralKinds<T extends TTemplateLiteralKind[]> = T extends [infer L extends TTemplateLiteralKind, ...infer R extends TTemplateLiteralKind[]] ? TFromTemplateLiteralKind<L> extends false ? false : TFromTemplateLiteralKinds<R> : true;
export declare function IsTemplateLiteralExpressionFinite(expression: Expression): boolean;
export type TIsTemplateLiteralFinite<T> = T extends TTemplateLiteral<infer U> ? TFromTemplateLiteralKinds<U> : false;
/** Returns true if this TemplateLiteral resolves to a finite set of values */
export declare function IsTemplateLiteralFinite<T extends TTemplateLiteral>(schema: T): boolean;
export {};
