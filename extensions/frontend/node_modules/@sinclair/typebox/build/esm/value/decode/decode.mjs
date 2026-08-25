import { HasTransform, TransformDecode, TransformDecodeCheckError } from '../transform/index.mjs';
import { Check } from '../check/index.mjs';
import { Errors } from '../../errors/index.mjs';
/** Decodes a value or throws if error */
export function Decode(...args) {
    const [schema, references, value] = args.length === 3 ? [args[0], args[1], args[2]] : [args[0], [], args[1]];
    if (!Check(schema, references, value))
        throw new TransformDecodeCheckError(schema, value, Errors(schema, references, value).First());
    return HasTransform(schema, references) ? TransformDecode(schema, references, value) : value;
}
