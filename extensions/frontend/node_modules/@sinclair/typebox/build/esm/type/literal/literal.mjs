import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Literal type */
export function Literal(value, options) {
    return CreateType({
        [Kind]: 'Literal',
        const: value,
        type: typeof value,
    }, options);
}
