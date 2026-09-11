"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Discard = Discard;
function DiscardKey(value, key) {
    const { [key]: _, ...rest } = value;
    return rest;
}
/** Discards property keys from the given value. This function returns a shallow Clone. */
function Discard(value, keys) {
    return keys.reduce((acc, key) => DiscardKey(acc, key), value);
}
