"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TemplateLiteralGenerateError = void 0;
exports.TemplateLiteralExpressionGenerate = TemplateLiteralExpressionGenerate;
exports.TemplateLiteralGenerate = TemplateLiteralGenerate;
const finite_1 = require("./finite");
const parse_1 = require("./parse");
const index_1 = require("../error/index");
// ------------------------------------------------------------------
// TemplateLiteralGenerateError
// ------------------------------------------------------------------
class TemplateLiteralGenerateError extends index_1.TypeBoxError {
}
exports.TemplateLiteralGenerateError = TemplateLiteralGenerateError;
// ------------------------------------------------------------------
// TemplateLiteralExpressionGenerate
// ------------------------------------------------------------------
// prettier-ignore
function* GenerateReduce(buffer) {
    if (buffer.length === 1)
        return yield* buffer[0];
    for (const left of buffer[0]) {
        for (const right of GenerateReduce(buffer.slice(1))) {
            yield `${left}${right}`;
        }
    }
}
// prettier-ignore
function* GenerateAnd(expression) {
    return yield* GenerateReduce(expression.expr.map((expr) => [...TemplateLiteralExpressionGenerate(expr)]));
}
// prettier-ignore
function* GenerateOr(expression) {
    for (const expr of expression.expr)
        yield* TemplateLiteralExpressionGenerate(expr);
}
// prettier-ignore
function* GenerateConst(expression) {
    return yield expression.const;
}
function* TemplateLiteralExpressionGenerate(expression) {
    return expression.type === 'and'
        ? yield* GenerateAnd(expression)
        : expression.type === 'or'
            ? yield* GenerateOr(expression)
            : expression.type === 'const'
                ? yield* GenerateConst(expression)
                : (() => {
                    throw new TemplateLiteralGenerateError('Unknown expression');
                })();
}
/** Generates a tuple of strings from the given TemplateLiteral. Returns an empty tuple if infinite. */
function TemplateLiteralGenerate(schema) {
    const expression = (0, parse_1.TemplateLiteralParseExact)(schema.pattern);
    // prettier-ignore
    return ((0, finite_1.IsTemplateLiteralExpressionFinite)(expression)
        ? [...TemplateLiteralExpressionGenerate(expression)]
        : []);
}
