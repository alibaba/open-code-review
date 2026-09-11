import { CreateType } from '../create/type.mjs';
import { Union } from '../union/index.mjs';
import { Never } from '../never/index.mjs';
import { ExtendsCheck, ExtendsResult } from '../extends/index.mjs';
import { ExtractFromMappedResult } from './extract-from-mapped-result.mjs';
import { ExtractFromTemplateLiteral } from './extract-from-template-literal.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsMappedResult, IsTemplateLiteral, IsUnion } from '../guard/kind.mjs';
function ExtractRest(L, R) {
    const extracted = L.filter((inner) => ExtendsCheck(inner, R) !== ExtendsResult.False);
    return extracted.length === 1 ? extracted[0] : Union(extracted);
}
/** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
export function Extract(L, R, options) {
    // overloads
    if (IsTemplateLiteral(L))
        return CreateType(ExtractFromTemplateLiteral(L, R), options);
    if (IsMappedResult(L))
        return CreateType(ExtractFromMappedResult(L, R), options);
    // prettier-ignore
    return CreateType(IsUnion(L) ? ExtractRest(L.anyOf, R) :
        ExtendsCheck(L, R) !== ExtendsResult.False ? L : Never(), options);
}
