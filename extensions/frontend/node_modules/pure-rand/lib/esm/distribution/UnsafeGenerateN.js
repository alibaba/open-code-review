export function unsafeGenerateN(rng, num) {
    var out = [];
    for (var idx = 0; idx != num; ++idx) {
        out.push(rng.unsafeNext());
    }
    return out;
}
