import { MappedResult } from '../mapped/index.mjs';
import { Literal } from '../literal/index.mjs';
import { Extends } from './extends.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function FromPropertyKey(K, U, L, R, options) {
    return {
        [K]: Extends(Literal(K), U, L, R, Clone(options))
    };
}
// prettier-ignore
function FromPropertyKeys(K, U, L, R, options) {
    return K.reduce((Acc, LK) => {
        return { ...Acc, ...FromPropertyKey(LK, U, L, R, options) };
    }, {});
}
// prettier-ignore
function FromMappedKey(K, U, L, R, options) {
    return FromPropertyKeys(K.keys, U, L, R, options);
}
// prettier-ignore
export function ExtendsFromMappedKey(T, U, L, R, options) {
    const P = FromMappedKey(T, U, L, R, options);
    return MappedResult(P);
}
