"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ReadonlyFromMappedResult = ReadonlyFromMappedResult;
const index_1 = require("../mapped/index");
const readonly_1 = require("./readonly");
// prettier-ignore
function FromProperties(K, F) {
    const Acc = {};
    for (const K2 of globalThis.Object.getOwnPropertyNames(K))
        Acc[K2] = (0, readonly_1.Readonly)(K[K2], F);
    return Acc;
}
// prettier-ignore
function FromMappedResult(R, F) {
    return FromProperties(R.properties, F);
}
// prettier-ignore
function ReadonlyFromMappedResult(R, F) {
    const P = FromMappedResult(R, F);
    return (0, index_1.MappedResult)(P);
}
