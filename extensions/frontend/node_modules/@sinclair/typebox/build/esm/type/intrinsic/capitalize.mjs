import { Intrinsic } from './intrinsic.mjs';
/** `[Json]` Intrinsic function to Capitalize LiteralString types */
export function Capitalize(T, options = {}) {
    return Intrinsic(T, 'Capitalize', options);
}
