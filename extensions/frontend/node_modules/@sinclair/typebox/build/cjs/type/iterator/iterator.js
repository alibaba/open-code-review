"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Iterator = Iterator;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates an Iterator type */
function Iterator(items, options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Iterator', type: 'Iterator', items }, options);
}
