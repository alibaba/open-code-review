import type { TSchema, SchemaOptions } from '../schema/index';
import type { Assert } from '../helpers/index';
import type { TUnion } from '../union/index';
import type { TLiteral } from '../literal/index';
import type { TInteger } from '../integer/index';
import type { TNumber } from '../number/index';
import type { TBigInt } from '../bigint/index';
import type { TString } from '../string/index';
import type { TBoolean } from '../boolean/index';
import type { TNever } from '../never/index';
import type { Static } from '../static/index';
import { type TTemplateLiteralSyntax } from './syntax';
import { EmptyString } from '../helpers/index';
import { Kind } from '../symbols/index';
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
