"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TemplateLiteral = TemplateLiteral;
const type_1 = require("../create/type");
const syntax_1 = require("./syntax");
const pattern_1 = require("./pattern");
const value_1 = require("../guard/value");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a TemplateLiteral type */
// prettier-ignore
function TemplateLiteral(unresolved, options) {
    const pattern = (0, value_1.IsString)(unresolved)
        ? (0, pattern_1.TemplateLiteralPattern)((0, syntax_1.TemplateLiteralSyntax)(unresolved))
        : (0, pattern_1.TemplateLiteralPattern)(unresolved);
    return (0, type_1.CreateType)({ [index_1.Kind]: 'TemplateLiteral', type: 'string', pattern }, options);
}
