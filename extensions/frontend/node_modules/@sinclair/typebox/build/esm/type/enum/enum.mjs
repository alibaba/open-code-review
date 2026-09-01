import { Literal } from '../literal/index.mjs';
import { Kind, Hint } from '../symbols/index.mjs';
import { Union } from '../union/index.mjs';
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
import { IsUndefined } from '../guard/value.mjs';
/** `[Json]` Creates a Enum type */
export function Enum(item, options) {
    if (IsUndefined(item))
        throw new Error('Enum undefined or empty');
    const values1 = globalThis.Object.getOwnPropertyNames(item)
        .filter((key) => isNaN(key))
        .map((key) => item[key]);
    const values2 = [...new Set(values1)];
    const anyOf = values2.map((value) => Literal(value));
    return Union(anyOf, { ...options, [Hint]: 'Enum' });
}
