import { MappedResult } from '../mapped/index.mjs';
import { Exclude } from './exclude.mjs';
// prettier-ignore
function FromProperties(P, U) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = Exclude(P[K2], U);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, T) {
    return FromProperties(R.properties, T);
}
// prettier-ignore
export function ExcludeFromMappedResult(R, T) {
    const P = FromMappedResult(R, T);
    return MappedResult(P);
}
