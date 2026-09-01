import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
export function UnionCreate(T, options) {
    return CreateType({ [Kind]: 'Union', anyOf: T }, options);
}
