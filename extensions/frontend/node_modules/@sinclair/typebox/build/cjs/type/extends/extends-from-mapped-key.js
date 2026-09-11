"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExtendsFromMappedKey = ExtendsFromMappedKey;
const index_1 = require("../mapped/index");
const index_2 = require("../literal/index");
const extends_1 = require("./extends");
const value_1 = require("../clone/value");
// prettier-ignore
function FromPropertyKey(K, U, L, R, options) {
    return {
        [K]: (0, extends_1.Extends)((0, index_2.Literal)(K), U, L, R, (0, value_1.Clone)(options))
    };
}
// prettier-ignore
function FromPropertyKeys(K, U, L, R, options) {
    return K.reduce((Acc, LK) => {
        return { ...Acc, ...FromPropertyKey(LK, U, L, R, options) };
    }, {});
}
// prettier-ignore
function FromMappedKey(K, U, L, R, options) {
    return FromPropertyKeys(K.keys, U, L, R, options);
}
// prettier-ignore
function ExtendsFromMappedKey(T, U, L, R, options) {
    const P = FromMappedKey(T, U, L, R, options);
    return (0, index_1.MappedResult)(P);
}
