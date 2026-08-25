import { CreateType } from '../create/type.mjs';
import { Never } from '../never/index.mjs';
import * as KindGuard from '../guard/kind.mjs';
/** `[JavaScript]` Extracts the ReturnType from the given Function type */
export function ReturnType(schema, options) {
    return (KindGuard.IsFunction(schema) ? CreateType(schema.returns, options) : Never(options));
}
