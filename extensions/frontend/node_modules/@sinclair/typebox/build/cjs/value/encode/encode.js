"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Encode = Encode;
const index_1 = require("../transform/index");
const index_2 = require("../check/index");
const index_3 = require("../../errors/index");
/** Encodes a value or throws if error */
function Encode(...args) {
    const [schema, references, value] = args.length === 3 ? [args[0], args[1], args[2]] : [args[0], [], args[1]];
    const encoded = (0, index_1.HasTransform)(schema, references) ? (0, index_1.TransformEncode)(schema, references, value) : value;
    if (!(0, index_2.Check)(schema, references, encoded))
        throw new index_1.TransformEncodeCheckError(schema, encoded, (0, index_3.Errors)(schema, references, encoded).First());
    return encoded;
}
