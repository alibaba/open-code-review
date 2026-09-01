"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TypeBoxError = void 0;
/** The base Error type thrown for all TypeBox exceptions  */
class TypeBoxError extends Error {
    constructor(message) {
        super(message);
    }
}
exports.TypeBoxError = TypeBoxError;
