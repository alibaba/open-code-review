import { PatternNumber, PatternString, PatternBoolean } from '../patterns/index.mjs';
import { Kind } from '../symbols/index.mjs';
import { TypeBoxError } from '../error/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsTemplateLiteral, IsUnion, IsNumber, IsInteger, IsBigInt, IsString, IsLiteral, IsBoolean } from '../guard/kind.mjs';
// ------------------------------------------------------------------
// TemplateLiteralPatternError
// ------------------------------------------------------------------
export class TemplateLiteralPatternError extends TypeBoxError {
}
// ------------------------------------------------------------------
// TemplateLiteralPattern
// ------------------------------------------------------------------
function Escape(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
// prettier-ignore
function Visit(schema, acc) {
    return (IsTemplateLiteral(schema) ? schema.pattern.slice(1, schema.pattern.length - 1) :
        IsUnion(schema) ? `(${schema.anyOf.map((schema) => Visit(schema, acc)).join('|')})` :
            IsNumber(schema) ? `${acc}${PatternNumber}` :
                IsInteger(schema) ? `${acc}${PatternNumber}` :
                    IsBigInt(schema) ? `${acc}${PatternNumber}` :
                        IsString(schema) ? `${acc}${PatternString}` :
                            IsLiteral(schema) ? `${acc}${Escape(schema.const.toString())}` :
                                IsBoolean(schema) ? `${acc}${PatternBoolean}` :
                                    (() => { throw new TemplateLiteralPatternError(`Unexpected Kind '${schema[Kind]}'`); })());
}
export function TemplateLiteralPattern(kinds) {
    return `^${kinds.map((schema) => Visit(schema, '')).join('')}\$`;
}
