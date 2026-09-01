import { TransformKind } from '../symbols/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsTransform } from '../guard/kind.mjs';
// ------------------------------------------------------------------
// TransformBuilders
// ------------------------------------------------------------------
export class TransformDecodeBuilder {
    constructor(schema) {
        this.schema = schema;
    }
    Decode(decode) {
        return new TransformEncodeBuilder(this.schema, decode);
    }
}
// prettier-ignore
export class TransformEncodeBuilder {
    constructor(schema, decode) {
        this.schema = schema;
        this.decode = decode;
    }
    EncodeTransform(encode, schema) {
        const Encode = (value) => schema[TransformKind].Encode(encode(value));
        const Decode = (value) => this.decode(schema[TransformKind].Decode(value));
        const Codec = { Encode: Encode, Decode: Decode };
        return { ...schema, [TransformKind]: Codec };
    }
    EncodeSchema(encode, schema) {
        const Codec = { Decode: this.decode, Encode: encode };
        return { ...schema, [TransformKind]: Codec };
    }
    Encode(encode) {
        return (IsTransform(this.schema) ? this.EncodeTransform(encode, this.schema) : this.EncodeSchema(encode, this.schema));
    }
}
/** `[Json]` Creates a Transform type */
export function Transform(schema) {
    return new TransformDecodeBuilder(schema);
}
