import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a Constructor type */
export function Constructor(parameters, returns, options) {
    return CreateType({ [Kind]: 'Constructor', type: 'Constructor', parameters, returns }, options);
}
