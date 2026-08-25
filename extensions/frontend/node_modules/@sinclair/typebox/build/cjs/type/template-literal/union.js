"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TemplateLiteralToUnion = TemplateLiteralToUnion;
const index_1 = require("../union/index");
const index_2 = require("../literal/index");
const generate_1 = require("./generate");
/** Returns a Union from the given TemplateLiteral */
function TemplateLiteralToUnion(schema) {
    const R = (0, generate_1.TemplateLiteralGenerate)(schema);
    const L = R.map((S) => (0, index_2.Literal)(S));
    return (0, index_1.UnionEvaluated)(L);
}
