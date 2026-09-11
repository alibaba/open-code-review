import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a String type */
export function String(options) {
    return CreateType({ [Kind]: 'String', type: 'string' }, options);
}
