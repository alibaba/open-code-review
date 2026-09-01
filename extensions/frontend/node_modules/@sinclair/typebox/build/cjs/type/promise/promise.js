"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Promise = Promise;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a Promise type */
function Promise(item, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Promise', type: 'Promise', item }, options);
}
