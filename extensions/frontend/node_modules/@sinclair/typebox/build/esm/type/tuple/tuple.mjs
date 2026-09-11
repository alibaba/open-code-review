import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Tuple type */
export function Tuple(types, options) {
    // prettier-ignore
    return CreateType(types.length > 0 ?
        { [Kind]: 'Tuple', type: 'array', items: types, additionalItems: false, minItems: types.length, maxItems: types.length } :
        { [Kind]: 'Tuple', type: 'array', minItems: types.length, maxItems: types.length }, options);
}
