import { MappedResult } from '../mapped/index.mjs';
import { Optional } from './optional.mjs';
// prettier-ignore
function FromProperties(P, F) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = Optional(P[K2], F);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, F) {
    return FromProperties(R.properties, F);
}
// prettier-ignore
export function OptionalFromMappedResult(R, F) {
    const P = FromMappedResult(R, F);
    return MappedResult(P);
}
