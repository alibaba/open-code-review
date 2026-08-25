import { CreateType } from '../create/type.mjs';
import { IsString } from '../guard/value.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[JavaScript]` Creates a RegExp type */
export function RegExp(unresolved, options) {
    const expr = IsString(unresolved) ? new globalThis.RegExp(unresolved) : unresolved;
    return CreateType({ [Kind]: 'RegExp', type: 'RegExp', source: expr.source, flags: expr.flags }, options);
}
