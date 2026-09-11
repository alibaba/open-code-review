"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Computed = Computed;
const index_1 = require("../create/index");
const symbols_1 = require("../symbols/symbols");
/** `[Internal]` Creates a deferred computed type. This type is used exclusively in modules to defer resolution of computable types that contain interior references  */
function Computed(target, parameters, options) {
    return (0, index_1.CreateType)({ [symbols_1.Kind]: 'Computed', target, parameters }, options);
}
