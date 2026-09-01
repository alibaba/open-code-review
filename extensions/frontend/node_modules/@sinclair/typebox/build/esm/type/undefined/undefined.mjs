import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a Undefined type */
export function Undefined(options) {
    return CreateType({ [Kind]: 'Undefined', type: 'undefined' }, options);
}
