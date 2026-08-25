"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TemplateLiteralFiniteError = void 0;
exports.IsTemplateLiteralExpressionFinite = IsTemplateLiteralExpressionFinite;
exports.IsTemplateLiteralFinite = IsTemplateLiteralFinite;
const parse_1 = require("./parse");
const index_1 = require("../error/index");
// ------------------------------------------------------------------
// TemplateLiteralFiniteError
// ------------------------------------------------------------------
class TemplateLiteralFiniteError extends index_1.TypeBoxError {
}
exports.TemplateLiteralFiniteError = TemplateLiteralFiniteError;
// ------------------------------------------------------------------
// IsTemplateLiteralFiniteCheck
// ------------------------------------------------------------------
// prettier-ignore
function IsNumberExpression(expression) {
    return (expression.type === 'or' &&
        expression.expr.length === 2 &&
        expression.expr[0].type === 'const' &&
        expression.expr[0].const === '0' &&
        expression.expr[1].type === 'const' &&
        expression.expr[1].const === '[1-9][0-9]*');
}
// prettier-ignore
function IsBooleanExpression(expression) {
    return (expression.type === 'or' &&
        expression.expr.length === 2 &&
        expression.expr[0].type === 'const' &&
        expression.expr[0].const === 'true' &&
        expression.expr[1].type === 'const' &&
        expression.expr[1].const === 'false');
}
// prettier-ignore
function IsStringExpression(expression) {
    return expression.type === 'const' && expression.const === '.*';
}
// ------------------------------------------------------------------
// IsTemplateLiteralExpressionFinite
// ------------------------------------------------------------------
// prettier-ignore
function IsTemplateLiteralExpressionFinite(expression) {
    return (IsNumberExpression(expression) || IsStringExpression(expression) ? false :
        IsBooleanExpression(expression) ? true :
            (expression.type === 'and') ? expression.expr.every((expr) => IsTemplateLiteralExpressionFinite(expr)) :
                (expression.type === 'or') ? expression.expr.every((expr) => IsTemplateLiteralExpressionFinite(expr)) :
                    (expression.type === 'const') ? true :
                        (() => { throw new TemplateLiteralFiniteError(`Unknown expression type`); })());
}
/** Returns true if this TemplateLiteral resolves to a finite set of values */
function IsTemplateLiteralFinite(schema) {
    const expression = (0, parse_1.TemplateLiteralParseExact)(schema.pattern);
    return IsTemplateLiteralExpressionFinite(expression);
}
