import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates an Unknown type */
export function Unknown(options) {
    return CreateType({ [Kind]: 'Unknown' }, options);
}
