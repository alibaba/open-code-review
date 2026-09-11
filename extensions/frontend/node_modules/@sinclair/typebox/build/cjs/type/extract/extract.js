"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Extract = Extract;
const type_1 = require("../create/type");
const index_1 = require("../union/index");
const index_2 = require("../never/index");
const index_3 = require("../extends/index");
const extract_from_mapped_result_1 = require("./extract-from-mapped-result");
const extract_from_template_literal_1 = require("./extract-from-template-literal");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
function ExtractRest(L, R) {
    const extracted = L.filter((inner) => (0, index_3.ExtendsCheck)(inner, R) !== index_3.ExtendsResult.False);
    return extracted.length === 1 ? extracted[0] : (0, index_1.Union)(extracted);
}
/** `[Json]` Constructs a type by extracting from type all union members that are assignable to union */
function Extract(L, R, options) {
    // overloads
    if ((0, kind_1.IsTemplateLiteral)(L))
        return (0, type_1.CreateType)((0, extract_from_template_literal_1.ExtractFromTemplateLiteral)(L, R), options);
    if ((0, kind_1.IsMappedResult)(L))
        return (0, type_1.CreateType)((0, extract_from_mapped_result_1.ExtractFromMappedResult)(L, R), options);
    // prettier-ignore
    return (0, type_1.CreateType)((0, kind_1.IsUnion)(L) ? ExtractRest(L.anyOf, R) :
        (0, index_3.ExtendsCheck)(L, R) !== index_3.ExtendsResult.False ? L : (0, index_2.Never)(), options);
}
