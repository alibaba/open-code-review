"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Number = Number;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a Number type */
function Number(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Number', type: 'number' }, options);
}
