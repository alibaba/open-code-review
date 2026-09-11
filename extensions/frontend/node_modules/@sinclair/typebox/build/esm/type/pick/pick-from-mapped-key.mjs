import { MappedResult } from '../mapped/index.mjs';
import { Pick } from './pick.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function FromPropertyKey(type, key, options) {
    return {
        [key]: Pick(type, [key], Clone(options))
    };
}
// prettier-ignore
function FromPropertyKeys(type, propertyKeys, options) {
    return propertyKeys.reduce((result, leftKey) => {
        return { ...result, ...FromPropertyKey(type, leftKey, options) };
    }, {});
}
// prettier-ignore
function FromMappedKey(type, mappedKey, options) {
    return FromPropertyKeys(type, mappedKey.keys, options);
}
// prettier-ignore
export function PickFromMappedKey(type, mappedKey, options) {
    const properties = FromMappedKey(type, mappedKey, options);
    return MappedResult(properties);
}
