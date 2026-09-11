"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TModule = void 0;
exports.Module = Module;
const index_1 = require("../create/index");
const index_2 = require("../symbols/index");
// ------------------------------------------------------------------
// Module Infrastructure Types
// ------------------------------------------------------------------
const compute_1 = require("./compute");
// ------------------------------------------------------------------
// Module
// ------------------------------------------------------------------
// prettier-ignore
class TModule {
    constructor($defs) {
        const computed = (0, compute_1.ComputeModuleProperties)($defs);
        const identified = this.WithIdentifiers(computed);
        this.$defs = identified;
    }
    /** `[Json]` Imports a Type by Key. */
    Import(key, options) {
        const $defs = { ...this.$defs, [key]: (0, index_1.CreateType)(this.$defs[key], options) };
        return (0, index_1.CreateType)({ [index_2.Kind]: 'Import', $defs, $ref: key });
    }
    // prettier-ignore
    WithIdentifiers($defs) {
        return globalThis.Object.getOwnPropertyNames($defs).reduce((result, key) => {
            return { ...result, [key]: { ...$defs[key], $id: key } };
        }, {});
    }
}
exports.TModule = TModule;
/** `[Json]` Creates a Type Definition Module. */
function Module(properties) {
    return new TModule(properties);
}
