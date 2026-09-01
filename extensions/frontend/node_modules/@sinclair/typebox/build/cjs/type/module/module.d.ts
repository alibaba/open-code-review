import { Kind } from '../symbols/index';
import { SchemaOptions, TSchema } from '../schema/index';
import { TProperties } from '../object/index';
import { Static } from '../static/index';
import { TComputeModuleProperties } from './compute';
import { TInferFromModuleKey } from './infer';
export interface TDefinitions<ModuleProperties extends TProperties> extends TSchema {
    static: {
        [K in keyof ModuleProperties]: Static<ModuleProperties[K]>;
    };
    $defs: ModuleProperties;
}
export interface TImport<ModuleProperties extends TProperties = {}, Key extends keyof ModuleProperties = keyof ModuleProperties> extends TSchema {
    [Kind]: 'Import';
    static: TInferFromModuleKey<ModuleProperties, Key>;
    $defs: ModuleProperties;
    $ref: Key;
}
export declare class TModule<ModuleProperties extends TProperties, ComputedModuleProperties extends TProperties = TComputeModuleProperties<ModuleProperties>> {
    private readonly $defs;
    constructor($defs: ModuleProperties);
    /** `[Json]` Imports a Type by Key. */
    Import<Key extends keyof ComputedModuleProperties>(key: Key, options?: SchemaOptions): TImport<ComputedModuleProperties, Key>;
    private WithIdentifiers;
}
/** `[Json]` Creates a Type Definition Module. */
export declare function Module<Properties extends TProperties>(properties: Properties): TModule<Properties>;
