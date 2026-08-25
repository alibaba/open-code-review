"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExcludeFromMappedResult = ExcludeFromMappedResult;
const index_1 = require("../mapped/index");
const exclude_1 = require("./exclude");
// prettier-ignore
function FromProperties(P, U) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = (0, exclude_1.Exclude)(P[K2], U);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, T) {
    return FromProperties(R.properties, T);
}
// prettier-ignore
function ExcludeFromMappedResult(R, T) {
    const P = FromMappedResult(R, T);
    return (0, index_1.MappedResult)(P);
}
