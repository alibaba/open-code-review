import { TypeBoxError } from '../../type/error/index.mjs';
import { Kind } from '../../type/symbols/index.mjs';
import { IsString } from '../guard/guard.mjs';
export class TypeDereferenceError extends TypeBoxError {
    constructor(schema) {
        super(`Unable to dereference schema with $id '${schema.$ref}'`);
        this.schema = schema;
    }
}
function Resolve(schema, references) {
    const target = references.find((target) => target.$id === schema.$ref);
    if (target === undefined)
        throw new TypeDereferenceError(schema);
    return Deref(target, references);
}
/** `[Internal]` Pushes a schema onto references if the schema has an $id and does not exist on references */
export function Pushref(schema, references) {
    if (!IsString(schema.$id) || references.some((target) => target.$id === schema.$id))
        return references;
    references.push(schema);
    return references;
}
/** `[Internal]` Dereferences a schema from the references array or throws if not found */
export function Deref(schema, references) {
    // prettier-ignore
    return (schema[Kind] === 'This' || schema[Kind] === 'Ref')
        ? Resolve(schema, references)
        : schema;
}
