"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.OmitFromMappedKey = OmitFromMappedKey;
const index_1 = require("../mapped/index");
const omit_1 = require("./omit");
const value_1 = require("../clone/value");
// prettier-ignore
function FromPropertyKey(type, key, options) {
    return { [key]: (0, omit_1.Omit)(type, [key], (0, value_1.Clone)(options)) };
}
// prettier-ignore
function FromPropertyKeys(type, propertyKeys, options) {
    return propertyKeys.reduce((Acc, LK) => {
        return { ...Acc, ...FromPropertyKey(type, LK, options) };
    }, {});
}
// prettier-ignore
function FromMappedKey(type, mappedKey, options) {
    return FromPropertyKeys(type, mappedKey.keys, options);
}
// prettier-ignore
function OmitFromMappedKey(type, mappedKey, options) {
    const properties = FromMappedKey(type, mappedKey, options);
    return (0, index_1.MappedResult)(properties);
}
