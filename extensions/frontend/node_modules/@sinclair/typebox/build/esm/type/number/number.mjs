import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Number type */
export function Number(options) {
    return CreateType({ [Kind]: 'Number', type: 'number' }, options);
}
