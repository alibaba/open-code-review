import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates an Array type */
export function Array(items, options) {
    return CreateType({ [Kind]: 'Array', type: 'array', items }, options);
}
