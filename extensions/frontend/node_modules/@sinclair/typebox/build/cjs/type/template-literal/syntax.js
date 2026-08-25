"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TemplateLiteralSyntax = TemplateLiteralSyntax;
const index_1 = require("../literal/index");
const index_2 = require("../boolean/index");
const index_3 = require("../bigint/index");
const index_4 = require("../number/index");
const index_5 = require("../string/index");
const index_6 = require("../union/index");
const index_7 = require("../never/index");
// ------------------------------------------------------------------
// SyntaxParsers
// ------------------------------------------------------------------
// prettier-ignore
function* FromUnion(syntax) {
    const trim = syntax.trim().replace(/"|'/g, '');
    return (trim === 'boolean' ? yield (0, index_2.Boolean)() :
        trim === 'number' ? yield (0, index_4.Number)() :
            trim === 'bigint' ? yield (0, index_3.BigInt)() :
                trim === 'string' ? yield (0, index_5.String)() :
                    yield (() => {
                        const literals = trim.split('|').map((literal) => (0, index_1.Literal)(literal.trim()));
                        return (literals.length === 0 ? (0, index_7.Never)() :
                            literals.length === 1 ? literals[0] :
                                (0, index_6.UnionEvaluated)(literals));
                    })());
}
// prettier-ignore
function* FromTerminal(syntax) {
    if (syntax[1] !== '{') {
        const L = (0, index_1.Literal)('$');
        const R = FromSyntax(syntax.slice(1));
        return yield* [L, ...R];
    }
    for (let i = 2; i < syntax.length; i++) {
        if (syntax[i] === '}') {
            const L = FromUnion(syntax.slice(2, i));
            const R = FromSyntax(syntax.slice(i + 1));
            return yield* [...L, ...R];
        }
    }
    yield (0, index_1.Literal)(syntax);
}
// prettier-ignore
function* FromSyntax(syntax) {
    for (let i = 0; i < syntax.length; i++) {
        if (syntax[i] === '$') {
            const L = (0, index_1.Literal)(syntax.slice(0, i));
            const R = FromTerminal(syntax.slice(i));
            return yield* [L, ...R];
        }
    }
    yield (0, index_1.Literal)(syntax);
}
/** Parses TemplateLiteralSyntax and returns a tuple of TemplateLiteralKinds */
function TemplateLiteralSyntax(syntax) {
    return [...FromSyntax(syntax)];
}
