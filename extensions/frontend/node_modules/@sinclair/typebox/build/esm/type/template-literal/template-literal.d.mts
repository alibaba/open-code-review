import type { TSchema, SchemaOptions } from '../schema/index.mjs';
import type { Assert } from '../helpers/index.mjs';
import type { TUnion } from '../union/index.mjs';
import type { TLiteral } from '../literal/index.mjs';
import type { TInteger } from '../integer/index.mjs';
import type { TNumber } from '../number/index.mjs';
import type { TBigInt } from '../bigint/index.mjs';
import type { TString } from '../string/index.mjs';
import type { TBoolean } from '../boolean/index.mjs';
import type { TNever } from '../never/index.mjs';
import type { Static } from '../static/index.mjs';
import { type TTemplateLiteralSyntax } from './syntax.mjs';
import { EmptyString } from '../helpers/index.mjs';
import { Kind } from '../symbols/index.mjs';
type TemplateLiteralStaticKind<T, Acc extends string> = T extends TUnion<infer U> ? {
    [K in keyof U]: TemplateLiteralStatic<Assert<[U[K]], TTemplateLiteralKind[]>, Acc>;
}[number] : T extends TTemplateLiteral ? `${Static<T>}` : T extends TLiteral<infer U> ? `${U}` : T extends TString ? `${string}` : T extends TNumber ? `${number}` : T extends TBigInt ? `${bigint}` : T extends TBoolean ? `${boolean}` : never;
type TemplateLiteralStatic<T extends TTemplateLiteralKind[], Acc extends string> = T extends [infer L, ...infer R] ? `${TemplateLiteralStaticKind<L, Acc>}${TemplateLiteralStatic<Assert<R, TTemplateLiteralKind[]>, Acc>}` : Acc;
export type TTemplateLiteralKind = TTemplateLiteral | TUnion | TLiteral | TInteger | TNumber | TBigInt | TString | TBoolean | TNever;
export interface TTemplateLiteral<T extends TTemplateLiteralKind[] = TTemplateLiteralKind[]> extends TSchema {
    [Kind]: 'TemplateLiteral';
    static: TemplateLiteralStatic<T, EmptyString>;
    type: 'string';
    pattern: string;
}
/** `[Json]` Creates a TemplateLiteral type from template dsl string */
export declare function TemplateLiteral<T extends string>(syntax: T, options?: SchemaOptions): TTemplateLiteralSyntax<T>;
/** `[Json]` Creates a TemplateLiteral type */
export declare function TemplateLiteral<T extends TTemplateLiteralKind[]>(kinds: [...T], options?: SchemaOptions): TTemplateLiteral<T>;
export {};
