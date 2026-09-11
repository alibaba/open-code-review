import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates an Argument Type. */
export function Argument(index) {
    return CreateType({ [Kind]: 'Argument', index });
}
