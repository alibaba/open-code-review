import { CreateType } from '../create/index.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates an Any type */
export function Any(options) {
    return CreateType({ [Kind]: 'Any' }, options);
}
