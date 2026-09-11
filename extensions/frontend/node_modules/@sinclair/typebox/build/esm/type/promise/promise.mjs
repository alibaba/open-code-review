import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a Promise type */
export function Promise(item, options) {
    return CreateType({ [Kind]: 'Promise', type: 'Promise', item }, options);
}
