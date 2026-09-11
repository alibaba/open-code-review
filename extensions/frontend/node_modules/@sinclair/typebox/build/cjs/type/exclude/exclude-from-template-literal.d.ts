import type { TSchema } from '../schema/index';
import { TExclude } from './exclude';
import { type TTemplateLiteral, type TTemplateLiteralToUnion } from '../template-literal/index';
export type TExcludeFromTemplateLiteral<L extends TTemplateLiteral, R extends TSchema> = (TExclude<TTemplateLiteralToUnion<L>, R>);
export declare function ExcludeFromTemplateLiteral<L extends TTemplateLiteral, R extends TSchema>(L: L, R: R): TExcludeFromTemplateLiteral<L, R>;
