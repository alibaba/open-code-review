import type { TSchema } from '../schema/index.mjs';
import { TExclude } from './exclude.mjs';
import { type TTemplateLiteral, type TTemplateLiteralToUnion } from '../template-literal/index.mjs';
export type TExcludeFromTemplateLiteral<L extends TTemplateLiteral, R extends TSchema> = (TExclude<TTemplateLiteralToUnion<L>, R>);
export declare function ExcludeFromTemplateLiteral<L extends TTemplateLiteral, R extends TSchema>(L: L, R: R): TExcludeFromTemplateLiteral<L, R>;
