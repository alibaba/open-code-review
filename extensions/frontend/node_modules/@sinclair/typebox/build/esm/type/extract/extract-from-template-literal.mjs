import { Extract } from './extract.mjs';
import { TemplateLiteralToUnion } from '../template-literal/index.mjs';
export function ExtractFromTemplateLiteral(L, R) {
    return Extract(TemplateLiteralToUnion(L), R);
}
