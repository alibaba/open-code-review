import { Tuple } from '../tuple/index.mjs';
import { Never } from '../never/index.mjs';
import * as KindGuard from '../guard/kind.mjs';
/** `[JavaScript]` Extracts the Parameters from the given Function type */
export function Parameters(schema, options) {
    return (KindGuard.IsFunction(schema) ? Tuple(schema.parameters, options) : Never());
}
