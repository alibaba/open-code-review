import { MappedResult } from '../mapped/index.mjs';
import { Intrinsic } from './intrinsic.mjs';
import { Literal } from '../literal/index.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function MappedIntrinsicPropertyKey(K, M, options) {
    return {
        [K]: Intrinsic(Literal(K), M, Clone(options))
    };
}
// prettier-ignore
function MappedIntrinsicPropertyKeys(K, M, options) {
    const result = K.reduce((Acc, L) => {
        return { ...Acc, ...MappedIntrinsicPropertyKey(L, M, options) };
    }, {});
    return result;
}
// prettier-ignore
function MappedIntrinsicProperties(T, M, options) {
    return MappedIntrinsicPropertyKeys(T['keys'], M, options);
}
// prettier-ignore
export function IntrinsicFromMappedKey(T, M, options) {
    const P = MappedIntrinsicProperties(T, M, options);
    return MappedResult(P);
}
