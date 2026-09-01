"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IntrinsicFromMappedKey = IntrinsicFromMappedKey;
const index_1 = require("../mapped/index");
const intrinsic_1 = require("./intrinsic");
const index_2 = require("../literal/index");
const value_1 = require("../clone/value");
// prettier-ignore
function MappedIntrinsicPropertyKey(K, M, options) {
    return {
        [K]: (0, intrinsic_1.Intrinsic)((0, index_2.Literal)(K), M, (0, value_1.Clone)(options))
    };
}
// prettier-ignore
function MappedIntrinsicPropertyKeys(K, M, options) {
    const result = K.reduce((Acc, L) => {
        return { ...Acc, ...MappedIntrinsicPropertyKey(L, M, options) };
    }, {});
    return result;
}
// prettier-ignore
function MappedIntrinsicProperties(T, M, options) {
    return MappedIntrinsicPropertyKeys(T['keys'], M, options);
}
// prettier-ignore
function IntrinsicFromMappedKey(T, M, options) {
    const P = MappedIntrinsicProperties(T, M, options);
    return (0, index_1.MappedResult)(P);
}
