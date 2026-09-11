import { CreateType } from '../create/type.mjs';
import { TemplateLiteralSyntax } from './syntax.mjs';
import { TemplateLiteralPattern } from './pattern.mjs';
import { IsString } from '../guard/value.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a TemplateLiteral type */
// prettier-ignore
export function TemplateLiteral(unresolved, options) {
    const pattern = IsString(unresolved)
        ? TemplateLiteralPattern(TemplateLiteralSyntax(unresolved))
        : TemplateLiteralPattern(unresolved);
    return CreateType({ [Kind]: 'TemplateLiteral', type: 'string', pattern }, options);
}
