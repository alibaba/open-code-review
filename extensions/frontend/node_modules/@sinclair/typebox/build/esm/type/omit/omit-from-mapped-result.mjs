import { MappedResult } from '../mapped/index.mjs';
import { Omit } from './omit.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function FromProperties(properties, propertyKeys, options) {
    const result = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(properties))
        result[K2] = Omit(properties[K2], propertyKeys, Clone(options));
    return result;
}
// prettier-ignore
function FromMappedResult(mappedResult, propertyKeys, options) {
    return FromProperties(mappedResult.properties, propertyKeys, options);
}
// prettier-ignore
export function OmitFromMappedResult(mappedResult, propertyKeys, options) {
    const properties = FromMappedResult(mappedResult, propertyKeys, options);
    return MappedResult(properties);
}
