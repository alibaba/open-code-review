import { Kind } from '../symbols/index.mjs';
import { CreateType } from '../create/index.mjs';
/** `[Json]` Creates a Boolean type */
export function Boolean(options) {
    return CreateType({ [Kind]: 'Boolean', type: 'boolean' }, options);
}
