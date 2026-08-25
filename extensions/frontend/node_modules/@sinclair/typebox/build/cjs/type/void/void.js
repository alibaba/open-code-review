"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Void = Void;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[JavaScript]` Creates a Void type */
function Void(options) {
    return (0, type_1.CreateType)({ [index_1.Kind]: 'Void', type: 'void' }, options);
}
