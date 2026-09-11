import { Intrinsic } from './intrinsic.mjs';
/** `[Json]` Intrinsic function to Lowercase LiteralString types */
export function Lowercase(T, options = {}) {
    return Intrinsic(T, 'Lowercase', options);
}
