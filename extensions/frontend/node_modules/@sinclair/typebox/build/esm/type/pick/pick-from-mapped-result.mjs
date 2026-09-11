import { MappedResult } from '../mapped/index.mjs';
import { Pick } from './pick.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function FromProperties(properties, propertyKeys, options) {
    const result = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(properties))
        result[K2] = Pick(properties[K2], propertyKeys, Clone(options));
    return result;
}
// prettier-ignore
function FromMappedResult(mappedResult, propertyKeys, options) {
    return FromProperties(mappedResult.properties, propertyKeys, options);
}
// prettier-ignore
export function PickFromMappedResult(mappedResult, propertyKeys, options) {
    const properties = FromMappedResult(mappedResult, propertyKeys, options);
    return MappedResult(properties);
}
