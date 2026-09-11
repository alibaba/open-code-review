import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Null type */
export function Null(options) {
    return CreateType({ [Kind]: 'Null', type: 'null' }, options);
}
