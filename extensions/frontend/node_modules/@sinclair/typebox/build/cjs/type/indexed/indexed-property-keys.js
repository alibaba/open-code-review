"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IndexPropertyKeys = IndexPropertyKeys;
const index_1 = require("../template-literal/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function FromTemplateLiteral(templateLiteral) {
    const keys = (0, index_1.TemplateLiteralGenerate)(templateLiteral);
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
function IndexPropertyKeys(type) {
    return [...new Set(((0, kind_1.IsTemplateLiteral)(type) ? FromTemplateLiteral(type) :
            (0, kind_1.IsUnion)(type) ? FromUnion(type.anyOf) :
                (0, kind_1.IsLiteral)(type) ? FromLiteral(type.const) :
                    (0, kind_1.IsNumber)(type) ? ['[number]'] :
                        (0, kind_1.IsInteger)(type) ? ['[number]'] :
                            []))];
}
