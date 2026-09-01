import { MappedResult } from '../mapped/index.mjs';
import { Partial } from './partial.mjs';
import { Clone } from '../clone/value.mjs';
// prettier-ignore
function FromProperties(K, options) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(K))
        Acc[K2] = Partial(K[K2], Clone(options));
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, options) {
    return FromProperties(R.properties, options);
}
// prettier-ignore
export function PartialFromMappedResult(R, options) {
    const P = FromMappedResult(R, options);
    return MappedResult(P);
}
