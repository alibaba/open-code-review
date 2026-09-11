import { MappedResult } from '../mapped/index.mjs';
import { IndexPropertyKeys } from './indexed-property-keys.mjs';
import { Index } from './index.mjs';
// prettier-ignore
function FromProperties(type, properties, options) {
    const result = {};
    for (const K2 of Object.getOwnPropertyNames(properties)) {
        result[K2] = Index(type, IndexPropertyKeys(properties[K2]), options);
    }
    return result;
}
// prettier-ignore
function FromMappedResult(type, mappedResult, options) {
    return FromProperties(type, mappedResult.properties, options);
}
// prettier-ignore
export function IndexFromMappedResult(type, mappedResult, options) {
    const properties = FromMappedResult(type, mappedResult, options);
    return MappedResult(properties);
}
