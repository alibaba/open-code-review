import { Intrinsic } from './intrinsic.mjs';
/** `[Json]` Intrinsic function to Uppercase LiteralString types */
export function Uppercase(T, options = {}) {
    return Intrinsic(T, 'Uppercase', options);
}
