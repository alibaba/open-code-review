import { TypeBoxError } from '../error/index.mjs';
import { CreateType } from '../create/type.mjs';
import { Kind } from '../symbols/index.mjs';
/** `[Json]` Creates a Ref type. The referenced type must contain a $id */
export function Ref(...args) {
    const [$ref, options] = typeof args[0] === 'string' ? [args[0], args[1]] : [args[0].$id, args[1]];
    if (typeof $ref !== 'string')
        throw new TypeBoxError('Ref: $ref must be a string');
    return CreateType({ [Kind]: 'Ref', $ref }, options);
}
