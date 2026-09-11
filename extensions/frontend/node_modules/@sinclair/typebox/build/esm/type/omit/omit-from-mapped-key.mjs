import { MappedResult } from '../mapped/index.mjs';
import { Omit } from './omit.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function FromPropertyKey(type, key, options) {
    return { [key]: Omit(type, [key], Clone(options)) };
}
// prettier-ignore
function FromPropertyKeys(type, propertyKeys, options) {
    return propertyKeys.reduce((Acc, LK) => {
        return { ...Acc, ...FromPropertyKey(type, LK, options) };
    }, {});
}
// prettier-ignore
function FromMappedKey(type, mappedKey, options) {
    return FromPropertyKeys(type, mappedKey.keys, options);
}
// prettier-ignore
export function OmitFromMappedKey(type, mappedKey, options) {
    const properties = FromMappedKey(type, mappedKey, options);
    return MappedResult(properties);
}
