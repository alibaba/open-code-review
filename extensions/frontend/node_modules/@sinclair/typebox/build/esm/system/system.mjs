import { TypeRegistry, FormatRegistry } from '../type/registry/index.mjs';
import { Unsafe } from '../type/unsafe/index.mjs';
import { Kind } from '../type/symbols/index.mjs';
import { TypeBoxError } from '../type/error/index.mjs';
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
export class TypeSystemDuplicateTypeKind extends TypeBoxError {
    constructor(kind) {
        super(`Duplicate type kind '${kind}' detected`);
    }
}
export class TypeSystemDuplicateFormat extends TypeBoxError {
    constructor(kind) {
        super(`Duplicate string format '${kind}' detected`);
    }
}
/** Creates user defined types and formats and provides overrides for value checking behaviours */
export var TypeSystem;
(function (TypeSystem) {
    /** Creates a new type */
    function Type(kind, check) {
        if (TypeRegistry.Has(kind))
            throw new TypeSystemDuplicateTypeKind(kind);
        TypeRegistry.Set(kind, check);
        return (options = {}) => Unsafe({ ...options, [Kind]: kind });
    }
    TypeSystem.Type = Type;
    /** Creates a new string format */
    function Format(format, check) {
        if (FormatRegistry.Has(format))
            throw new TypeSystemDuplicateFormat(format);
        FormatRegistry.Set(format, check);
        return format;
    }
    TypeSystem.Format = Format;
})(TypeSystem || (TypeSystem = {}));
