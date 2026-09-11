import { IndexFromPropertyKeys } from '../indexed/indexed.mjs';
import { KeyOfPropertyKeys } from './keyof-property-keys.mjs';
/**
 * `[Utility]` Resolves an array of keys and schemas from the given schema. This method is faster
 * than obtaining the keys and resolving each individually via indexing. This method was written
 * accellerate Intersect and Union encoding.
 */
export function KeyOfPropertyEntries(schema) {
    const keys = KeyOfPropertyKeys(schema);
    const schemas = IndexFromPropertyKeys(schema, keys);
    return keys.map((_, index) => [keys[index], schemas[index]]);
}
