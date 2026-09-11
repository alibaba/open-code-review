export function unsafeSkipN(rng, num) {
    for (var idx = 0; idx != num; ++idx) {
        rng.unsafeNext();
    }
}
