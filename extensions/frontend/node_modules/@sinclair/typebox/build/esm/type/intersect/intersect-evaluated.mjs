import { OptionalKind } from '../symbols/index.mjs';
import { CreateType } from '../create/type.mjs';
import { Discard } from '../discard/index.mjs';
import { Never } from '../never/index.mjs';
import { Optional } from '../optional/index.mjs';
import { IntersectCreate } from './intersect-create.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsOptional, IsTransform } from '../guard/kind.mjs';
// prettier-ignore
function IsIntersectOptional(types) {
    return types.every(left => IsOptional(left));
}
// prettier-ignore
function RemoveOptionalFromType(type) {
    return (Discard(type, [OptionalKind]));
}
// prettier-ignore
function RemoveOptionalFromRest(types) {
    return types.map(left => IsOptional(left) ? RemoveOptionalFromType(left) : left);
}
// prettier-ignore
function ResolveIntersect(types, options) {
    return (IsIntersectOptional(types)
        ? Optional(IntersectCreate(RemoveOptionalFromRest(types), options))
        : IntersectCreate(RemoveOptionalFromRest(types), options));
}
/** `[Json]` Creates an evaluated Intersect type */
export function IntersectEvaluated(types, options = {}) {
    if (types.length === 1)
        return CreateType(types[0], options);
    if (types.length === 0)
        return Never(options);
    if (types.some((schema) => IsTransform(schema)))
        throw new Error('Cannot intersect transform types');
    return ResolveIntersect(types, options);
}
