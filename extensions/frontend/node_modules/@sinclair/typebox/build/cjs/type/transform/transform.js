"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TransformEncodeBuilder = exports.TransformDecodeBuilder = void 0;
exports.Transform = Transform;
const index_1 = require("../symbols/index");
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
const kind_1 = require("../guard/kind");
// ------------------------------------------------------------------
// TransformBuilders
// ------------------------------------------------------------------
class TransformDecodeBuilder {
    constructor(schema) {
        this.schema = schema;
    }
    Decode(decode) {
        return new TransformEncodeBuilder(this.schema, decode);
    }
}
exports.TransformDecodeBuilder = TransformDecodeBuilder;
// prettier-ignore
class TransformEncodeBuilder {
    constructor(schema, decode) {
        this.schema = schema;
        this.decode = decode;
    }
    EncodeTransform(encode, schema) {
        const Encode = (value) => schema[index_1.TransformKind].Encode(encode(value));
        const Decode = (value) => this.decode(schema[index_1.TransformKind].Decode(value));
        const Codec = { Encode: Encode, Decode: Decode };
        return { ...schema, [index_1.TransformKind]: Codec };
    }
    EncodeSchema(encode, schema) {
        const Codec = { Decode: this.decode, Encode: encode };
        return { ...schema, [index_1.TransformKind]: Codec };
    }
    Encode(encode) {
        return ((0, kind_1.IsTransform)(this.schema) ? this.EncodeTransform(encode, this.schema) : this.EncodeSchema(encode, this.schema));
    }
}
exports.TransformEncodeBuilder = TransformEncodeBuilder;
/** `[Json]` Creates a Transform type */
function Transform(schema) {
    return new TransformDecodeBuilder(schema);
}
