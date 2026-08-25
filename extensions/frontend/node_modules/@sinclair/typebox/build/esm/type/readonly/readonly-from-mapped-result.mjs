import { MappedResult } from '../mapped/index.mjs';
import { Readonly } from './readonly.mjs';
// prettier-ignore
function FromProperties(K, F) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(K))
        Acc[K2] = Readonly(K[K2], F);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, F) {
    return FromProperties(R.properties, F);
}
// prettier-ignore
export function ReadonlyFromMappedResult(R, F) {
    const P = FromMappedResult(R, F);
    return MappedResult(P);
}
