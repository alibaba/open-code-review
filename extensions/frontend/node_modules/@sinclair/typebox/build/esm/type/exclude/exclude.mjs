import { CreateType } from '../create/type.mjs';
import { Union } from '../union/index.mjs';
import { Never } from '../never/index.mjs';
import { ExtendsCheck, ExtendsResult } from '../extends/index.mjs';
import { ExcludeFromMappedResult } from './exclude-from-mapped-result.mjs';
import { ExcludeFromTemplateLiteral } from './exclude-from-template-literal.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsMappedResult, IsTemplateLiteral, IsUnion } from '../guard/kind.mjs';
function ExcludeRest(L, R) {
    const excluded = L.filter((inner) => ExtendsCheck(inner, R) === ExtendsResult.False);
    return excluded.length === 1 ? excluded[0] : Union(excluded);
}
/** `[Json]` Constructs a type by excluding from unionType all union members that are assignable to excludedMembers */
export function Exclude(L, R, options = {}) {
    // overloads
    if (IsTemplateLiteral(L))
        return CreateType(ExcludeFromTemplateLiteral(L, R), options);
    if (IsMappedResult(L))
        return CreateType(ExcludeFromMappedResult(L, R), options);
    // prettier-ignore
    return CreateType(IsUnion(L) ? ExcludeRest(L.anyOf, R) :
        ExtendsCheck(L, R) !== ExtendsResult.False ? Never() : L, options);
}
