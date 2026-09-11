import { UnionEvaluated } from '../union/index.mjs';
import { Literal } from '../literal/index.mjs';
import { TemplateLiteralGenerate } from './generate.mjs';
/** Returns a Union from the given TemplateLiteral */
export function TemplateLiteralToUnion(schema) {
    const R = TemplateLiteralGenerate(schema);
    const L = R.map((S) => Literal(S));
    return UnionEvaluated(L);
}
