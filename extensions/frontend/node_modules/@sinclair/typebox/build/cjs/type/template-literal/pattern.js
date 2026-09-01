"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TemplateLiteralPatternError = void 0;
exports.TemplateLiteralPattern = TemplateLiteralPattern;
const index_1 = require("../patterns/index");
const index_2 = require("../symbols/index");
const index_3 = require("../error/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// ------------------------------------------------------------------
// TemplateLiteralPatternError
// ------------------------------------------------------------------
class TemplateLiteralPatternError extends index_3.TypeBoxError {
}
exports.TemplateLiteralPatternError = TemplateLiteralPatternError;
// ------------------------------------------------------------------
// TemplateLiteralPattern
// ------------------------------------------------------------------
function Escape(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
// prettier-ignore
function Visit(schema, acc) {
    return ((0, kind_1.IsTemplateLiteral)(schema) ? schema.pattern.slice(1, schema.pattern.length - 1) :
        (0, kind_1.IsUnion)(schema) ? `(${schema.anyOf.map((schema) => Visit(schema, acc)).join('|')})` :
            (0, kind_1.IsNumber)(schema) ? `${acc}${index_1.PatternNumber}` :
                (0, kind_1.IsInteger)(schema) ? `${acc}${index_1.PatternNumber}` :
                    (0, kind_1.IsBigInt)(schema) ? `${acc}${index_1.PatternNumber}` :
                        (0, kind_1.IsString)(schema) ? `${acc}${index_1.PatternString}` :
                            (0, kind_1.IsLiteral)(schema) ? `${acc}${Escape(schema.const.toString())}` :
                                (0, kind_1.IsBoolean)(schema) ? `${acc}${index_1.PatternBoolean}` :
                                    (() => { throw new TemplateLiteralPatternError(`Unexpected Kind '${schema[index_2.Kind]}'`); })());
}
function TemplateLiteralPattern(kinds) {
    return `^${kinds.map((schema) => Visit(schema, '')).join('')}\$`;
}
