import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates an Integer type */
export function Integer(options) {
    return CreateType({ [Kind]: 'Integer', type: 'integer' }, options);
}
