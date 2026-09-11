"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Composite = Composite;
const index_1 = require("../intersect/index");
const index_2 = require("../indexed/index");
const index_3 = require("../keyof/index");
const index_4 = require("../object/index");
const index_5 = require("../sets/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// prettier-ignore
function CompositeKeys(T) {
    const Acc = [];
    for (const L of T)
        Acc.push(...(0, index_3.KeyOfPropertyKeys)(L));
    return (0, index_5.SetDistinct)(Acc);
}
// prettier-ignore
function FilterNever(T) {
    return T.filter(L => !(0, kind_1.IsNever)(L));
}
// prettier-ignore
function CompositeProperty(T, K) {
    const Acc = [];
    for (const L of T)
        Acc.push(...(0, index_2.IndexFromPropertyKeys)(L, [K]));
    return FilterNever(Acc);
}
// prettier-ignore
function CompositeProperties(T, K) {
    const Acc = {};
    for (const L of K) {
        Acc[L] = (0, index_1.IntersectEvaluated)(CompositeProperty(T, L));
    }
    return Acc;
}
// prettier-ignore
function Composite(T, options) {
    const K = CompositeKeys(T);
    const P = CompositeProperties(T, K);
    const R = (0, index_4.Object)(P, options);
    return R;
}
