import { IntersectEvaluated } from '../intersect/index.mjs';
import { IndexFromPropertyKeys } from '../indexed/index.mjs';
import { KeyOfPropertyKeys } from '../keyof/index.mjs';
import { Object as _Object_ } from '../object/index.mjs';
import { SetDistinct } from '../sets/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsNever } from '../guard/kind.mjs';
// prettier-ignore
function CompositeKeys(T) {
    const Acc = [];
    for (const L of T)
        Acc.push(...KeyOfPropertyKeys(L));
    return SetDistinct(Acc);
}
// prettier-ignore
function FilterNever(T) {
    return T.filter(L => !IsNever(L));
}
// prettier-ignore
function CompositeProperty(T, K) {
    const Acc = [];
    for (const L of T)
        Acc.push(...IndexFromPropertyKeys(L, [K]));
    return FilterNever(Acc);
}
// prettier-ignore
function CompositeProperties(T, K) {
    const Acc = {};
    for (const L of K) {
        Acc[L] = IntersectEvaluated(CompositeProperty(T, L));
    }
    return Acc;
}
// prettier-ignore
export function Composite(T, options) {
    const K = CompositeKeys(T);
    const P = CompositeProperties(T, K);
    const R = _Object_(P, options);
    return R;
}
