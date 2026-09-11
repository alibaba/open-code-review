import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a Void type */
export function Void(options) {
    return CreateType({ [Kind]: 'Void', type: 'void' }, options);
}
