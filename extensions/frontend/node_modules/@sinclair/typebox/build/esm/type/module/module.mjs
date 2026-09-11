import { CreateType } from '../create/index.mjs';
import { Kind } from '../symbols/index.mjs';
// ------------------------------------------------------------------
// Module Infrastructure Types
// ------------------------------------------------------------------
import { ComputeModuleProperties } from './compute.mjs';
// ------------------------------------------------------------------
// Module
// ------------------------------------------------------------------
// prettier-ignore
export class TModule {
    constructor($defs) {
        const computed = ComputeModuleProperties($defs);
        const identified = this.WithIdentifiers(computed);
        this.$defs = identified;
    }
    /** `[Json]` Imports a Type by Key. */
    Import(key, options) {
        const $defs = { ...this.$defs, [key]: CreateType(this.$defs[key], options) };
        return CreateType({ [Kind]: 'Import', $defs, $ref: key });
    }
    // prettier-ignore
    WithIdentifiers($defs) {
        return globalThis.Object.getOwnPropertyNames($defs).reduce((result, key) => {
            return { ...result, [key]: { ...$defs[key], $id: key } };
        }, {});
    }
}
/** `[Json]` Creates a Type Definition Module. */
export function Module(properties) {
    return new TModule(properties);
}
