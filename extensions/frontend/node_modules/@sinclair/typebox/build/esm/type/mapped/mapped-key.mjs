import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
// prettier-ignore
export function MappedKey(T) {
    return CreateType({
        [Kind]: 'MappedKey',
        keys: T
    });
}
