import { Kind } from '../symbols/index.mjs';
import { CreateType } from '../create/type.mjs';
/** `[JavaScript]` Creates a Date type */
export function Date(options) {
    return CreateType({ [Kind]: 'Date', type: 'Date' }, options);
}
