import { Kind } from '../symbols/index.mjs';
import { CreateType } from '../create/type.mjs';
/** `[JavaScript]` Creates a AsyncIterator type */
export function AsyncIterator(items, options) {
    return CreateType({ [Kind]: 'AsyncIterator', type: 'AsyncIterator', items }, options);
}
