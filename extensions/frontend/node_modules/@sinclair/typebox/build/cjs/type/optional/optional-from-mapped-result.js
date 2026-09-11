"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.OptionalFromMappedResult = OptionalFromMappedResult;
const index_1 = require("../mapped/index");
const optional_1 = require("./optional");
// prettier-ignore
function FromProperties(P, F) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = (0, optional_1.Optional)(P[K2], F);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, F) {
    return FromProperties(R.properties, F);
}
// prettier-ignore
function OptionalFromMappedResult(R, F) {
    const P = FromMappedResult(R, F);
    return (0, index_1.MappedResult)(P);
}
