import { Exclude } from './exclude.mjs';
import { TemplateLiteralToUnion } from '../template-literal/index.mjs';
export function ExcludeFromTemplateLiteral(L, R) {
    return Exclude(TemplateLiteralToUnion(L), R);
}
