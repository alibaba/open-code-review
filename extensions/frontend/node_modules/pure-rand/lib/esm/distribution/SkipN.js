import { unsafeSkipN } from './UnsafeSkipN.js';
export function skipN(rng, num) {
    var nextRng = rng.clone();
    unsafeSkipN(nextRng, num);
    return nextRng;
}
