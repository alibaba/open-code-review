import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Unsafe type that will infers as the generic argument T */
export function Unsafe(options = {}) {
    return CreateType({ [Kind]: options[Kind] ?? 'Unsafe' }, options);
}
