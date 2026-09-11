"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Lowercase = Lowercase;
const intrinsic_1 = require("./intrinsic");
/** `[Json]` Intrinsic function to Lowercase LiteralString types */
function Lowercase(T, options = {}) {
    return (0, intrinsic_1.Intrinsic)(T, 'Lowercase', options);
}
