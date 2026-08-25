"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Recursive = Recursive;
const type_1 = require("../clone/type");
const type_2 = require("../create/type");
const value_1 = require("../guard/value");
const index_1 = require("../symbols/index");
// Auto Tracked For Recursive Types without ID's
let Ordinal = 0;
/** `[Json]` Creates a Recursive type */
function Recursive(callback, options = {}) {
    if ((0, value_1.IsUndefined)(options.$id))
        options.$id = `T${Ordinal++}`;
    const thisType = (0, type_1.CloneType)(callback({ [index_1.Kind]: 'This', $ref: `${options.$id}` }));
    thisType.$id = options.$id;
    // prettier-ignore
    return (0, type_2.CreateType)({ [index_1.Hint]: 'Recursive', ...thisType }, options);
}
