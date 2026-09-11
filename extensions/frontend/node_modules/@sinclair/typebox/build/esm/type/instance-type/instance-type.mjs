import { CreateType } from '../create/type.mjs';
import { Never } from '../never/index.mjs';
import * as KindGuard from '../guard/kind.mjs';
/** `[JavaScript]` Extracts the InstanceType from the given Constructor type */
export function InstanceType(schema, options) {
    return (KindGuard.IsConstructor(schema) ? CreateType(schema.returns, options) : Never(options));
}
