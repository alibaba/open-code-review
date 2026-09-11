import { Readonly } from '../readonly/index.mjs';
import { Optional } from '../optional/index.mjs';
/** `[Json]` Creates a Readonly and Optional property */
export function ReadonlyOptional(schema) {
    return Readonly(Optional(schema));
}
