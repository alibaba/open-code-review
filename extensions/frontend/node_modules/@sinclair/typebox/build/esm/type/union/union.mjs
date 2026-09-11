import { Never } from '../never/index.mjs';
import { CreateType } from '../create/type.mjs';
import { UnionCreate } from './union-create.mjs';
/** `[Json]` Creates a Union type */
export function Union(types, options) {
    // prettier-ignore
    return (types.length === 0 ? Never(options) :
        types.length === 1 ? CreateType(types[0], options) :
            UnionCreate(types, options));
}
