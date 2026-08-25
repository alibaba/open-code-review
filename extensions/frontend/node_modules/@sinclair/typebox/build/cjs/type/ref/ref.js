"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Ref = Ref;
const index_1 = require("../error/index");
const type_1 = require("../create/type");
const index_2 = require("../symbols/index");
/** `[Json]` Creates a Ref type. The referenced type must contain a $id */
function Ref(...args) {
    const [$ref, options] = typeof args[0] === 'string' ? [args[0], args[1]] : [args[0].$id, args[1]];
    if (typeof $ref !== 'string')
        throw new index_1.TypeBoxError('Ref: $ref must be a string');
    return (0, type_1.CreateType)({ [index_2.Kind]: 'Ref', $ref }, options);
}
