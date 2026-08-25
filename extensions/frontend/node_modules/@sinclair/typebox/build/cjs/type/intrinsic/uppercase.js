"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Uppercase = Uppercase;
const intrinsic_1 = require("./intrinsic");
/** `[Json]` Intrinsic function to Uppercase LiteralString types */
function Uppercase(T, options = {}) {
    return (0, intrinsic_1.Intrinsic)(T, 'Uppercase', options);
}
