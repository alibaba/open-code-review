"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.unsafeSkipN = unsafeSkipN;
function unsafeSkipN(rng, num) {
    for (var idx = 0; idx != num; ++idx) {
        rng.unsafeNext();
    }
}
