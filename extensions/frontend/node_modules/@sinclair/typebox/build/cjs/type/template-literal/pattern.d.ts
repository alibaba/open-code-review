import type { TTemplateLiteralKind } from './index';
import { TypeBoxError } from '../error/index';
export declare class TemplateLiteralPatternError extends TypeBoxError {
}
export declare function TemplateLiteralPattern(kinds: TTemplateLiteralKind[]): string;
