import { Index } from './indexed.mjs';
import { MappedResult } from '../mapped/index.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function MappedIndexPropertyKey(type, key, options) {
    return { [key]: Index(type, [key], Clone(options)) };
}
// prettier-ignore
function MappedIndexPropertyKeys(type, propertyKeys, options) {
    return propertyKeys.reduce((result, left) => {
        return { ...result, ...MappedIndexPropertyKey(type, left, options) };
    }, {});
}
// prettier-ignore
function MappedIndexProperties(type, mappedKey, options) {
    return MappedIndexPropertyKeys(type, mappedKey.keys, options);
}
// prettier-ignore
export function IndexFromMappedKey(type, mappedKey, options) {
    const properties = MappedIndexProperties(type, mappedKey, options);
    return MappedResult(properties);
}
