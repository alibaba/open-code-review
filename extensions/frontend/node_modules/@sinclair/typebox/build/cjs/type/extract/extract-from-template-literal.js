"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExtractFromTemplateLiteral = ExtractFromTemplateLiteral;
const extract_1 = require("./extract");
const index_1 = require("../template-literal/index");
function ExtractFromTemplateLiteral(L, R) {
    return (0, extract_1.Extract)((0, index_1.TemplateLiteralToUnion)(L), R);
}
