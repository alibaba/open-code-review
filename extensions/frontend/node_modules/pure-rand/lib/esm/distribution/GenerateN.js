import { unsafeGenerateN } from './UnsafeGenerateN.js';
export function generateN(rng, num) {
    var nextRng = rng.clone();
    var out = unsafeGenerateN(nextRng, num);
    return [out, nextRng];
}
