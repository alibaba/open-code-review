import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a Function type */
export function Function(parameters, returns, options) {
    return CreateType({ [Kind]: 'Function', type: 'Function', parameters, returns }, options);
}
