import { MappedResult } from '../mapped/index.mjs';
import { Extract } from './extract.mjs';
// prettier-ignore
function FromProperties(P, T) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = Extract(P[K2], T);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, T) {
    return FromProperties(R.properties, T);
}
// prettier-ignore
export function ExtractFromMappedResult(R, T) {
    const P = FromMappedResult(R, T);
    return MappedResult(P);
}
