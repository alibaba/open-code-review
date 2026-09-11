import { HasTransform, TransformEncode, TransformEncodeCheckError } from '../transform/index.mjs';
import { Check } from '../check/index.mjs';
import { Errors } from '../../errors/index.mjs';
/** Encodes a value or throws if error */
export function Encode(...args) {
    const [schema, references, value] = args.length === 3 ? [args[0], args[1], args[2]] : [args[0], [], args[1]];
    const encoded = HasTransform(schema, references) ? TransformEncode(schema, references, value) : value;
    if (!Check(schema, references, encoded))
        throw new TransformEncodeCheckError(schema, encoded, Errors(schema, references, encoded).First());
    return encoded;
}
