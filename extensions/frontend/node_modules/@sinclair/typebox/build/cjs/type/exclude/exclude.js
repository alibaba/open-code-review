"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Exclude = Exclude;
const type_1 = require("../create/type");
const index_1 = require("../union/index");
const index_2 = require("../never/index");
const index_3 = require("../extends/index");
const exclude_from_mapped_result_1 = require("./exclude-from-mapped-result");
const exclude_from_template_literal_1 = require("./exclude-from-template-literal");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
function ExcludeRest(L, R) {
    const excluded = L.filter((inner) => (0, index_3.ExtendsCheck)(inner, R) === index_3.ExtendsResult.False);
    return excluded.length === 1 ? excluded[0] : (0, index_1.Union)(excluded);
}
/** `[Json]` Constructs a type by excluding from unionType all union members that are assignable to excludedMembers */
function Exclude(L, R, options = {}) {
    // overloads
    if ((0, kind_1.IsTemplateLiteral)(L))
        return (0, type_1.CreateType)((0, exclude_from_template_literal_1.ExcludeFromTemplateLiteral)(L, R), options);
    if ((0, kind_1.IsMappedResult)(L))
        return (0, type_1.CreateType)((0, exclude_from_mapped_result_1.ExcludeFromMappedResult)(L, R), options);
    // prettier-ignore
    return (0, type_1.CreateType)((0, kind_1.IsUnion)(L) ? ExcludeRest(L.anyOf, R) :
        (0, index_3.ExtendsCheck)(L, R) !== index_3.ExtendsResult.False ? (0, index_2.Never)() : L, options);
}
