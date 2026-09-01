"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.IntersectCreate = IntersectCreate;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// ------------------------------------------------------------------
// IntersectCreate
// ------------------------------------------------------------------
// prettier-ignore
function IntersectCreate(T, options = {}) {
    const allObjects = T.every((schema) => (0, kind_1.IsObject)(schema));
    const clonedUnevaluatedProperties = (0, kind_1.IsSchema)(options.unevaluatedProperties)
        ? { unevaluatedProperties: options.unevaluatedProperties }
        : {};
    return (0, type_1.CreateType)((options.unevaluatedProperties === false || (0, kind_1.IsSchema)(options.unevaluatedProperties) || allObjects
        ? { ...clonedUnevaluatedProperties, [index_1.Kind]: 'Intersect', type: 'object', allOf: T }
        : { ...clonedUnevaluatedProperties, [index_1.Kind]: 'Intersect', allOf: T }), options);
}
