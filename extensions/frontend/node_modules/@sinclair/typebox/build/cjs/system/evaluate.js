"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Evaluate = Evaluate;
/**
 * Evaluates code in the current environment. This function matches centralized
 * evaluation as implemented in TypeBox 1.x.
 */
function Evaluate(...args) {
    return new globalThis.Function(...args);
}
