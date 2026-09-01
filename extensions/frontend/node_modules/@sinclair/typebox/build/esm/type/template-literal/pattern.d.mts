import type { TTemplateLiteralKind } from './index.mjs';
import { TypeBoxError } from '../error/index.mjs';
export declare class TemplateLiteralPatternError extends TypeBoxError {
}
export declare function TemplateLiteralPattern(kinds: TTemplateLiteralKind[]): string;
