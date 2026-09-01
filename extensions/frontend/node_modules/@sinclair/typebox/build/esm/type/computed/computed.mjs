import { CreateType } from '../create/index.mjs';
import { Kind } from '../symbols/symbols.mjs';
/** `[Internal]` Creates a deferred computed type. This type is used exclusively in modules to defer resolution of computable types that contain interior references  */
export function Computed(target, parameters, options) {
    return CreateType({ [Kind]: 'Computed', target, parameters }, options);
}
