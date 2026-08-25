import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a Uint8Array type */
export function Uint8Array(options) {
    return CreateType({ [Kind]: 'Uint8Array', type: 'Uint8Array' }, options);
}
