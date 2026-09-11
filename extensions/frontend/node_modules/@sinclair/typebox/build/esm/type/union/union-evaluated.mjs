import { CreateType } from '../create/type.mjs';
import { OptionalKind } from '../symbols/index.mjs';
import { Discard } from '../discard/index.mjs';
import { Never } from '../never/index.mjs';
import { Optional } from '../optional/index.mjs';
import { UnionCreate } from './union-create.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsOptional } from '../guard/kind.mjs';
// prettier-ignore
function IsUnionOptional(types) {
    return types.some(type => IsOptional(type));
}
// prettier-ignore
function RemoveOptionalFromRest(types) {
    return types.map(left => IsOptional(left) ? RemoveOptionalFromType(left) : left);
}
// prettier-ignore
function RemoveOptionalFromType(T) {
    return (Discard(T, [OptionalKind]));
}
// prettier-ignore
function ResolveUnion(types, options) {
    const isOptional = IsUnionOptional(types);
    return (isOptional
        ? Optional(UnionCreate(RemoveOptionalFromRest(types), options))
        : UnionCreate(RemoveOptionalFromRest(types), options));
}
/** `[Json]` Creates an evaluated Union type */
export function UnionEvaluated(T, options) {
    // prettier-ignore
    return (T.length === 1 ? CreateType(T[0], options) :
        T.length === 0 ? Never(options) :
            ResolveUnion(T, options));
}
