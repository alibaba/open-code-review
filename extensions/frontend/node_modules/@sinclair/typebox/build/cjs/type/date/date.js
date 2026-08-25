"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Date = Date;
const index_1 = require("../symbols/index");
const type_1 = require("../create/type");
/** `[JavaScript]` Creates a Date type */
function Date(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Date', type: 'Date' }, options);
}
