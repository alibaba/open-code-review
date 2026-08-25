import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a Symbol type */
export function Symbol(options) {
    return CreateType({ [Kind]: 'Symbol', type: 'symbol' }, options);
}
