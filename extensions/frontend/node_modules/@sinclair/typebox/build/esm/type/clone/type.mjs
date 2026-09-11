import { Clone } from './value.mjs';
/** Clones a Rest */
export function CloneRest(schemas) {
    return schemas.map((schema) => CloneType(schema));
}
/** Clones a Type */
export function CloneType(schema, options) {
    return options === undefined ? Clone(schema) : Clone({ ...options, ...schema });
}
