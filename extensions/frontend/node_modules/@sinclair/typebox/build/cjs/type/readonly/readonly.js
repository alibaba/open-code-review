"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Readonly = Readonly;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
const index_2 = require("../discard/index");
const readonly_from_mapped_result_1 = require("./readonly-from-mapped-result");
const kind_1 = require("../guard/kind");
function RemoveReadonly(schema) {
    return (0, type_1.CreateType)((0, index_2.Discard)(schema, [index_1.ReadonlyKind]));
}
function AddReadonly(schema) {
    return (0, type_1.CreateType)({ ...schema, [index_1.ReadonlyKind]: 'Readonly' });
}
// prettier-ignore
function ReadonlyWithFlag(schema, F) {
    return (F === false
        ? RemoveReadonly(schema)
        : AddReadonly(schema));
}
/** `[Json]` Creates a Readonly property */
function Readonly(schema, enable) {
    const F = enable ?? true;
    return (0, kind_1.IsMappedResult)(schema) ? (0, readonly_from_mapped_result_1.ReadonlyFromMappedResult)(schema, F) : ReadonlyWithFlag(schema, F);
}
