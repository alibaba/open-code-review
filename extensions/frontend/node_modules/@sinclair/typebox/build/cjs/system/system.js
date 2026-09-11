"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.TypeSystem = exports.TypeSystemDuplicateFormat = exports.TypeSystemDuplicateTypeKind = void 0;
const index_1 = require("../type/registry/index");
const index_2 = require("../type/unsafe/index");
const index_3 = require("../type/symbols/index");
const index_4 = require("../type/error/index");
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
class TypeSystemDuplicateTypeKind extends index_4.TypeBoxError {
    constructor(kind) {
        super(`Duplicate type kind '${kind}' detected`);
    }
}
exports.TypeSystemDuplicateTypeKind = TypeSystemDuplicateTypeKind;
class TypeSystemDuplicateFormat extends index_4.TypeBoxError {
    constructor(kind) {
        super(`Duplicate string format '${kind}' detected`);
    }
}
exports.TypeSystemDuplicateFormat = TypeSystemDuplicateFormat;
/** Creates user defined types and formats and provides overrides for value checking behaviours */
var TypeSystem;
(function (TypeSystem) {
    /** Creates a new type */
    function Type(kind, check) {
        if (index_1.TypeRegistry.Has(kind))
            throw new TypeSystemDuplicateTypeKind(kind);
        index_1.TypeRegistry.Set(kind, check);
        return (options = {}) => (0, index_2.Unsafe)({ ...options, [index_3.Kind]: kind });
    }
    TypeSystem.Type = Type;
    /** Creates a new string format */
    function Format(format, check) {
        if (index_1.FormatRegistry.Has(format))
            throw new TypeSystemDuplicateFormat(format);
        index_1.FormatRegistry.Set(format, check);
        return format;
    }
    TypeSystem.Format = Format;
})(TypeSystem || (exports.TypeSystem = TypeSystem = {}));
