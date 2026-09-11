"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.PickFromMappedKey = PickFromMappedKey;
const index_1 = require("../mapped/index");
const pick_1 = require("./pick");
const value_1 = require("../clone/value");
// prettier-ignore
function FromPropertyKey(type, key, options) {
    return {
        [key]: (0, pick_1.Pick)(type, [key], (0, value_1.Clone)(options))
    };
}
// prettier-ignore
function FromPropertyKeys(type, propertyKeys, options) {
    return propertyKeys.reduce((result, leftKey) => {
        return { ...result, ...FromPropertyKey(type, leftKey, options) };
    }, {});
}
// prettier-ignore
function FromMappedKey(type, mappedKey, options) {
    return FromPropertyKeys(type, mappedKey.keys, options);
}
// prettier-ignore
function PickFromMappedKey(type, mappedKey, options) {
    const properties = FromMappedKey(type, mappedKey, options);
    return (0, index_1.MappedResult)(properties);
}
