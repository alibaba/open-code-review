"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Capitalize = Capitalize;
const intrinsic_1 = require("./intrinsic");
/** `[Json]` Intrinsic function to Capitalize LiteralString types */
function Capitalize(T, options = {}) {
    return (0, intrinsic_1.Intrinsic)(T, 'Capitalize', options);
}
