import type { TSchema } from '../schema/index.mjs';
import { type TExtract } from './extract.mjs';
import { type TTemplateLiteral, type TTemplateLiteralToUnion } from '../template-literal/index.mjs';
export type TExtractFromTemplateLiteral<L extends TTemplateLiteral, R extends TSchema> = (TExtract<TTemplateLiteralToUnion<L>, R>);
export declare function ExtractFromTemplateLiteral<L extends TTemplateLiteral, R extends TSchema>(L: L, R: R): TExtractFromTemplateLiteral<L, R>;
