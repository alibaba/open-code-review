"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Intrinsic = Intrinsic;
const type_1 = require("../create/type");
const index_1 = require("../template-literal/index");
const intrinsic_from_mapped_key_1 = require("./intrinsic-from-mapped-key");
const index_2 = require("../literal/index");
const index_3 = require("../union/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
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
    const expression = (0, index_1.TemplateLiteralParseExact)(schema.pattern);
    const finite = (0, index_1.IsTemplateLiteralExpressionFinite)(expression);
    if (!finite)
        return { ...schema, pattern: FromLiteralValue(schema.pattern, mode) };
    const strings = [...(0, index_1.TemplateLiteralExpressionGenerate)(expression)];
    const literals = strings.map((value) => (0, index_2.Literal)(value));
    const mapped = FromRest(literals, mode);
    const union = (0, index_3.Union)(mapped);
    return (0, index_1.TemplateLiteral)([union], options);
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
function Intrinsic(schema, mode, options = {}) {
    // prettier-ignore
    return (
    // Intrinsic-Mapped-Inference
    (0, kind_1.IsMappedKey)(schema) ? (0, intrinsic_from_mapped_key_1.IntrinsicFromMappedKey)(schema, mode, options) :
        // Standard-Inference
        (0, kind_1.IsTemplateLiteral)(schema) ? FromTemplateLiteral(schema, mode, options) :
            (0, kind_1.IsUnion)(schema) ? (0, index_3.Union)(FromRest(schema.anyOf, mode), options) :
                (0, kind_1.IsLiteral)(schema) ? (0, index_2.Literal)(FromLiteralValue(schema.const, mode), options) :
                    // Default Type
                    (0, type_1.CreateType)(schema, options));
}
