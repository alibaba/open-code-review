import { CreateType } from '../create/type.mjs';
import { Never } from '../never/index.mjs';
import { IntersectCreate } from './intersect-create.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsTransform } from '../guard/kind.mjs';
/** `[Json]` Creates an evaluated Intersect type */
export function Intersect(types, options) {
    if (types.length === 1)
        return CreateType(types[0], options);
    if (types.length === 0)
        return Never(options);
    if (types.some((schema) => IsTransform(schema)))
        throw new Error('Cannot intersect transform types');
    return IntersectCreate(types, options);
}
