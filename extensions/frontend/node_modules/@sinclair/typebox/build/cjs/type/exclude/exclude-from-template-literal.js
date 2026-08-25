"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.ExcludeFromTemplateLiteral = ExcludeFromTemplateLiteral;
const exclude_1 = require("./exclude");
const index_1 = require("../template-literal/index");
function ExcludeFromTemplateLiteral(L, R) {
    return (0, exclude_1.Exclude)((0, index_1.TemplateLiteralToUnion)(L), R);
}
