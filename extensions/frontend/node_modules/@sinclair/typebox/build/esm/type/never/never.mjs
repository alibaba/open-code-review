import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Never type */
export function Never(options) {
    return CreateType({ [Kind]: 'Never', not: {} }, options);
}
