import { Literal } from '../literal/index.mjs';
import { Boolean } from '../boolean/index.mjs';
import { BigInt } from '../bigint/index.mjs';
import { Number } from '../number/index.mjs';
import { String } from '../string/index.mjs';
import { UnionEvaluated } from '../union/index.mjs';
import { Never } from '../never/index.mjs';
// ------------------------------------------------------------------
// SyntaxParsers
// ------------------------------------------------------------------
// prettier-ignore
function* FromUnion(syntax) {
    const trim = syntax.trim().replace(/"|'/g, '');
    return (trim === 'boolean' ? yield Boolean() :
        trim === 'number' ? yield Number() :
            trim === 'bigint' ? yield BigInt() :
                trim === 'string' ? yield String() :
                    yield (() => {
                        const literals = trim.split('|').map((literal) => Literal(literal.trim()));
                        return (literals.length === 0 ? Never() :
                            literals.length === 1 ? literals[0] :
                                UnionEvaluated(literals));
                    })());
}
// prettier-ignore
function* FromTerminal(syntax) {
    if (syntax[1] !== '{') {
        const L = Literal('$');
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
    yield Literal(syntax);
}
// prettier-ignore
function* FromSyntax(syntax) {
    for (let i = 0; i < syntax.length; i++) {
        if (syntax[i] === '$') {
            const L = Literal(syntax.slice(0, i));
            const R = FromTerminal(syntax.slice(i));
            return yield* [L, ...R];
        }
    }
    yield Literal(syntax);
}
/** Parses TemplateLiteralSyntax and returns a tuple of TemplateLiteralKinds */
export function TemplateLiteralSyntax(syntax) {
    return [...FromSyntax(syntax)];
}
