"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExtractFromMappedResult = ExtractFromMappedResult;
const index_1 = require("../mapped/index");
const extract_1 = require("./extract");
// prettier-ignore
function FromProperties(P, T) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(P))
        Acc[K2] = (0, extract_1.Extract)(P[K2], T);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, T) {
    return FromProperties(R.properties, T);
}
// prettier-ignore
function ExtractFromMappedResult(R, T) {
    const P = FromMappedResult(R, T);
    return (0, index_1.MappedResult)(P);
}
