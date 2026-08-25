// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsIntersect, IsUnion, IsTuple } from '../guard/kind.mjs';
// prettier-ignore
function RestResolve(T) {
    return (IsIntersect(T) ? T.allOf :
        IsUnion(T) ? T.anyOf :
            IsTuple(T) ? T.items ?? [] :
                []);
}
/** `[Json]` Extracts interior Rest elements from Tuple, Intersect and Union types */
export function Rest(T) {
    return RestResolve(T);
}
