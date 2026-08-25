import { TypeBoxError } from '../error/index.mjs';
// ------------------------------------------------------------------
// TemplateLiteralParserError
// ------------------------------------------------------------------
export class TemplateLiteralParserError extends TypeBoxError {
}
// -------------------------------------------------------------------
// Unescape
//
// Unescape for these control characters specifically. Note that this
// function is only called on non union group content, and where we
// still want to allow the user to embed control characters in that
// content. For review.
// -------------------------------------------------------------------
// prettier-ignore
function Unescape(pattern) {
    return pattern
        .replace(/\\\$/g, '$')
        .replace(/\\\*/g, '*')
        .replace(/\\\^/g, '^')
        .replace(/\\\|/g, '|')
        .replace(/\\\(/g, '(')
        .replace(/\\\)/g, ')');
}
// -------------------------------------------------------------------
// Control Characters
// -------------------------------------------------------------------
function IsNonEscaped(pattern, index, char) {
    return pattern[index] === char && pattern.charCodeAt(index - 1) !== 92;
}
function IsOpenParen(pattern, index) {
    return IsNonEscaped(pattern, index, '(');
}
function IsCloseParen(pattern, index) {
    return IsNonEscaped(pattern, index, ')');
}
function IsSeparator(pattern, index) {
    return IsNonEscaped(pattern, index, '|');
}
// -------------------------------------------------------------------
// Control Groups
// -------------------------------------------------------------------
function IsGroup(pattern) {
    if (!(IsOpenParen(pattern, 0) && IsCloseParen(pattern, pattern.length - 1)))
        return false;
    let count = 0;
    for (let index = 0; index < pattern.length; index++) {
        if (IsOpenParen(pattern, index))
            count += 1;
        if (IsCloseParen(pattern, index))
            count -= 1;
        if (count === 0 && index !== pattern.length - 1)
            return false;
    }
    return true;
}
// prettier-ignore
function InGroup(pattern) {
    return pattern.slice(1, pattern.length - 1);
}
// prettier-ignore
function IsPrecedenceOr(pattern) {
    let count = 0;
    for (let index = 0; index < pattern.length; index++) {
        if (IsOpenParen(pattern, index))
            count += 1;
        if (IsCloseParen(pattern, index))
            count -= 1;
        if (IsSeparator(pattern, index) && count === 0)
            return true;
    }
    return false;
}
// prettier-ignore
function IsPrecedenceAnd(pattern) {
    for (let index = 0; index < pattern.length; index++) {
        if (IsOpenParen(pattern, index))
            return true;
    }
    return false;
}
// prettier-ignore
function Or(pattern) {
    let [count, start] = [0, 0];
    const expressions = [];
    for (let index = 0; index < pattern.length; index++) {
        if (IsOpenParen(pattern, index))
            count += 1;
        if (IsCloseParen(pattern, index))
            count -= 1;
        if (IsSeparator(pattern, index) && count === 0) {
            const range = pattern.slice(start, index);
            if (range.length > 0)
                expressions.push(TemplateLiteralParse(range));
            start = index + 1;
        }
    }
    const range = pattern.slice(start);
    if (range.length > 0)
        expressions.push(TemplateLiteralParse(range));
    if (expressions.length === 0)
        return { type: 'const', const: '' };
    if (expressions.length === 1)
        return expressions[0];
    return { type: 'or', expr: expressions };
}
// prettier-ignore
function And(pattern) {
    function Group(value, index) {
        if (!IsOpenParen(value, index))
            throw new TemplateLiteralParserError(`TemplateLiteralParser: Index must point to open parens`);
        let count = 0;
        for (let scan = index; scan < value.length; scan++) {
            if (IsOpenParen(value, scan))
                count += 1;
            if (IsCloseParen(value, scan))
                count -= 1;
            if (count === 0)
                return [index, scan];
        }
        throw new TemplateLiteralParserError(`TemplateLiteralParser: Unclosed group parens in expression`);
    }
    function Range(pattern, index) {
        for (let scan = index; scan < pattern.length; scan++) {
            if (IsOpenParen(pattern, scan))
                return [index, scan];
        }
        return [index, pattern.length];
    }
    const expressions = [];
    for (let index = 0; index < pattern.length; index++) {
        if (IsOpenParen(pattern, index)) {
            const [start, end] = Group(pattern, index);
            const range = pattern.slice(start, end + 1);
            expressions.push(TemplateLiteralParse(range));
            index = end;
        }
        else {
            const [start, end] = Range(pattern, index);
            const range = pattern.slice(start, end);
            if (range.length > 0)
                expressions.push(TemplateLiteralParse(range));
            index = end - 1;
        }
    }
    return ((expressions.length === 0) ? { type: 'const', const: '' } :
        (expressions.length === 1) ? expressions[0] :
            { type: 'and', expr: expressions });
}
// ------------------------------------------------------------------
// TemplateLiteralParse
// ------------------------------------------------------------------
/** Parses a pattern and returns an expression tree */
export function TemplateLiteralParse(pattern) {
    // prettier-ignore
    return (IsGroup(pattern) ? TemplateLiteralParse(InGroup(pattern)) :
        IsPrecedenceOr(pattern) ? Or(pattern) :
            IsPrecedenceAnd(pattern) ? And(pattern) :
                { type: 'const', const: Unescape(pattern) });
}
// ------------------------------------------------------------------
// TemplateLiteralParseExact
// ------------------------------------------------------------------
/** Parses a pattern and strips forward and trailing ^ and $ */
export function TemplateLiteralParseExact(pattern) {
    return TemplateLiteralParse(pattern.slice(1, pattern.length - 1));
}
