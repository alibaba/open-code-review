import { CreateType } from '../create/type.mjs';
import { TemplateLiteral, TemplateLiteralParseExact, IsTemplateLiteralExpressionFinite, TemplateLiteralExpressionGenerate } from '../template-literal/index.mjs';
import { IntrinsicFromMappedKey } from './intrinsic-from-mapped-key.mjs';
import { Literal } from '../literal/index.mjs';
import { Union } from '../union/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsMappedKey, IsTemplateLiteral, IsUnion, IsLiteral } from '../guard/kind.mjs';
// ------------------------------------------------------------------
// Apply
// ------------------------------------------------------------------
function ApplyUncapitalize(value) {
    const [first, rest] = [value.slice(0, 1), value.slice(1)];
    return [first.toLowerCase(), rest].join('');
}
function ApplyCapitalize(value) {
    const [first, rest] = [value.slice(0, 1), value.slice(1)];
    return [first.toUpperCase(), rest].join('');
}
function ApplyUppercase(value) {
    return value.toUpperCase();
}
function ApplyLowercase(value) {
    return value.toLowerCase();
}
function FromTemplateLiteral(schema, mode, options) {
    // note: template literals require special runtime handling as they are encoded in string patterns.
    // This diverges from the mapped type which would otherwise map on the template literal kind.
    const expression = TemplateLiteralParseExact(schema.pattern);
    const finite = IsTemplateLiteralExpressionFinite(expression);
    if (!finite)
        return { ...schema, pattern: FromLiteralValue(schema.pattern, mode) };
    const strings = [...TemplateLiteralExpressionGenerate(expression)];
    const literals = strings.map((value) => Literal(value));
    const mapped = FromRest(literals, mode);
    const union = Union(mapped);
    return TemplateLiteral([union], options);
}
// prettier-ignore
function FromLiteralValue(value, mode) {
    return (typeof value === 'string' ? (mode === 'Uncapitalize' ? ApplyUncapitalize(value) :
        mode === 'Capitalize' ? ApplyCapitalize(value) :
            mode === 'Uppercase' ? ApplyUppercase(value) :
                mode === 'Lowercase' ? ApplyLowercase(value) :
                    value) : value.toString());
}
// prettier-ignore
function FromRest(T, M) {
    return T.map(L => Intrinsic(L, M));
}
/** Applies an intrinsic string manipulation to the given type. */
export function Intrinsic(schema, mode, options = {}) {
    // prettier-ignore
    return (
    // Intrinsic-Mapped-Inference
    IsMappedKey(schema) ? IntrinsicFromMappedKey(schema, mode, options) :
        // Standard-Inference
        IsTemplateLiteral(schema) ? FromTemplateLiteral(schema, mode, options) :
            IsUnion(schema) ? Union(FromRest(schema.anyOf, mode), options) :
                IsLiteral(schema) ? Literal(FromLiteralValue(schema.const, mode), options) :
                    // Default Type
                    CreateType(schema, options));
}
