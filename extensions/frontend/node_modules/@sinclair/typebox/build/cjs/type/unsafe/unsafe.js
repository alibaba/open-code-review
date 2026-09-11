"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Unsafe = Unsafe;
const type_1 = require("../create/type");
const index_1 = require("../symbols/index");
/** `[Json]` Creates a Unsafe type that will infers as the generic argument T */
function Unsafe(options = {}) {
    return (0, type_1.CreateType)({ [index_1.Kind]: options[index_1.Kind] ?? 'Unsafe' }, options);
}
