"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Decode = Decode;
const index_1 = require("../transform/index");
const index_2 = require("../check/index");
const index_3 = require("../../errors/index");
/** Decodes a value or throws if error */
function Decode(...args) {
    const [schema, references, value] = args.length === 3 ? [args[0], args[1], args[2]] : [args[0], [], args[1]];
    if (!(0, index_2.Check)(schema, references, value))
        throw new index_1.TransformDecodeCheckError(schema, value, (0, index_3.Errors)(schema, references, value).First());
    return (0, index_1.HasTransform)(schema, references) ? (0, index_1.TransformDecode)(schema, references, value) : value;
}
