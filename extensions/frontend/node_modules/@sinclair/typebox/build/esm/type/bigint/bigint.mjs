import { Kind } from '../symbols/index.mjs';
import { CreateType } from '../create/index.mjs';
/** `[JavaScript]` Creates a BigInt type */
export function BigInt(options) {
    return CreateType({ [Kind]: 'BigInt', type: 'bigint' }, options);
}
