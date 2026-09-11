"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IndexFromMappedKey = IndexFromMappedKey;
const indexed_1 = require("./indexed");
const index_1 = require("../mapped/index");
const value_1 = require("../clone/value");
// prettier-ignore
function MappedIndexPropertyKey(type, key, options) {
    return { [key]: (0, indexed_1.Index)(type, [key], (0, value_1.Clone)(options)) };
}
// prettier-ignore
function MappedIndexPropertyKeys(type, propertyKeys, options) {
    return propertyKeys.reduce((result, left) => {
        return { ...result, ...MappedIndexPropertyKey(type, left, options) };
    }, {});
}
// prettier-ignore
function MappedIndexProperties(type, mappedKey, options) {
    return MappedIndexPropertyKeys(type, mappedKey.keys, options);
}
// prettier-ignore
function IndexFromMappedKey(type, mappedKey, options) {
    const properties = MappedIndexProperties(type, mappedKey, options);
    return (0, index_1.MappedResult)(properties);
}
