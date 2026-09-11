import { TemplateLiteralGenerate } from '../template-literal/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsTemplateLiteral, IsUnion, IsLiteral, IsNumber, IsInteger } from '../guard/kind.mjs';
// prettier-ignore
function FromTemplateLiteral(templateLiteral) {
    const keys = TemplateLiteralGenerate(templateLiteral);
    return keys.map(key => key.toString());
}
// prettier-ignore
function FromUnion(types) {
    const result = [];
    for (const type of types)
        result.push(...IndexPropertyKeys(type));
    return result;
}
// prettier-ignore
function FromLiteral(literalValue) {
    return ([literalValue.toString()] // TS 5.4 observes TLiteralValue as not having a toString()
    );
}
/** Returns a tuple of PropertyKeys derived from the given TSchema */
// prettier-ignore
export function IndexPropertyKeys(type) {
    return [...new Set((IsTemplateLiteral(type) ? FromTemplateLiteral(type) :
            IsUnion(type) ? FromUnion(type.anyOf) :
                IsLiteral(type) ? FromLiteral(type.const) :
                    IsNumber(type) ? ['[number]'] :
                        IsInteger(type) ? ['[number]'] :
                            []))];
}
