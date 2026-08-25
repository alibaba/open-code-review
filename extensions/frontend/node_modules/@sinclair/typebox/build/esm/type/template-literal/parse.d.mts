import { TypeBoxError } from '../error/index.mjs';
export declare class TemplateLiteralParserError extends TypeBoxError {
}
export type Expression = ExpressionAnd | ExpressionOr | ExpressionConst;
export type ExpressionConst = {
    type: 'const';
    const: string;
};
export type ExpressionAnd = {
    type: 'and';
    expr: Expression[];
};
export type ExpressionOr = {
    type: 'or';
    expr: Expression[];
};
/** Parses a pattern and returns an expression tree */
export declare function TemplateLiteralParse(pattern: string): Expression;
/** Parses a pattern and strips forward and trailing ^ and $ */
export declare function TemplateLiteralParseExact(pattern: string): Expression;
