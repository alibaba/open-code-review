import { Intrinsic } from './intrinsic.mjs';
/** `[Json]` Intrinsic function to Uncapitalize LiteralString types */
export function Uncapitalize(T, options = {}) {
    return Intrinsic(T, 'Uncapitalize', options);
}
