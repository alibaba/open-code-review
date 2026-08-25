import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
// prettier-ignore
export function MappedResult(properties) {
    return CreateType({
        [Kind]: 'MappedResult',
        properties
    });
}
