import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Not type */
export function Not(type, options) {
    return CreateType({ [Kind]: 'Not', not: type }, options);
}
