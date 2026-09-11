import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates an Iterator type */
export function Iterator(items, options) {
    return CreateType({ [Kind]: 'Iterator', type: 'Iterator', items }, options);
}
