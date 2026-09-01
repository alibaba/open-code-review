import { MappedResult } from '../mapped/index.mjs';
import { Required } from './required.mjs';
// prettier-ignore
function FromProperties(P, options) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = Required(P[K2], options);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, options) {
    return FromProperties(R.properties, options);
}
// prettier-ignore
export function RequiredFromMappedResult(R, options) {
    const P = FromMappedResult(R, options);
    return MappedResult(P);
}
