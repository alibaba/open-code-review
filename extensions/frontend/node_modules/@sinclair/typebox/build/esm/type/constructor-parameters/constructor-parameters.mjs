import { Tuple } from '../tuple/index.mjs';
import { Never } from '../never/index.mjs';
import * as KindGuard from '../guard/kind.mjs';
/** `[JavaScript]` Extracts the ConstructorParameters from the given Constructor type */
export function ConstructorParameters(schema, options) {
    return (KindGuard.IsConstructor(schema) ? Tuple(schema.parameters, options) : Never(options));
}
