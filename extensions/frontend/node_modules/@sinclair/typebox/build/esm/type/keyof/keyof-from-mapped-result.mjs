import { MappedResult } from '../mapped/index.mjs';
import { KeyOf } from './keyof.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function FromProperties(properties, options) {
    const result = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(properties))
        result[K2] = KeyOf(properties[K2], Clone(options));
    return result;
}
// prettier-ignore
function FromMappedResult(mappedResult, options) {
    return FromProperties(mappedResult.properties, options);
}
// prettier-ignore
export function KeyOfFromMappedResult(mappedResult, options) {
    const properties = FromMappedResult(mappedResult, options);
    return MappedResult(properties);
}
