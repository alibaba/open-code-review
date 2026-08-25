import { HasPropertyKey } from '../guard/index.mjs';
import { Check } from '../check/index.mjs';
import { Clone } from '../clone/index.mjs';
import { Deref, Pushref } from '../deref/index.mjs';
import { TemplateLiteralGenerate, IsTemplateLiteralFinite } from '../../type/template-literal/index.mjs';
import { TypeRegistry } from '../../type/registry/index.mjs';
import { Kind } from '../../type/symbols/index.mjs';
import { TypeBoxError } from '../../type/error/index.mjs';
import { IsFunction } from '../guard/guard.mjs';
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
export class ValueCreateError extends TypeBoxError {
    constructor(schema, message) {
        super(message);
        this.schema = schema;
    }
}
// ------------------------------------------------------------------
// Default
// ------------------------------------------------------------------
function FromDefault(value) {
    return IsFunction(value) ? value() : Clone(value);
}
// ------------------------------------------------------------------
// Create
// ------------------------------------------------------------------
function FromAny(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return {};
    }
}
function FromArgument(schema, references) {
    return {};
}
function FromArray(schema, references) {
    if (schema.uniqueItems === true && !HasPropertyKey(schema, 'default')) {
        throw new ValueCreateError(schema, 'Array with the uniqueItems constraint requires a default value');
    }
    else if ('contains' in schema && !HasPropertyKey(schema, 'default')) {
        throw new ValueCreateError(schema, 'Array with the contains constraint requires a default value');
    }
    else if ('default' in schema) {
        return FromDefault(schema.default);
    }
    else if (schema.minItems !== undefined) {
        return Array.from({ length: schema.minItems }).map((item) => {
            return Visit(schema.items, references);
        });
    }
    else {
        return [];
    }
}
function FromAsyncIterator(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return (async function* () { })();
    }
}
function FromBigInt(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return BigInt(0);
    }
}
function FromBoolean(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return false;
    }
}
function FromConstructor(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        const value = Visit(schema.returns, references);
        if (typeof value === 'object' && !Array.isArray(value)) {
            return class {
                constructor() {
                    for (const [key, val] of Object.entries(value)) {
                        const self = this;
                        self[key] = val;
                    }
                }
            };
        }
        else {
            return class {
            };
        }
    }
}
function FromDate(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else if (schema.minimumTimestamp !== undefined) {
        return new Date(schema.minimumTimestamp);
    }
    else {
        return new Date();
    }
}
function FromFunction(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return () => Visit(schema.returns, references);
    }
}
function FromImport(schema, references) {
    const definitions = globalThis.Object.values(schema.$defs);
    const target = schema.$defs[schema.$ref];
    return Visit(target, [...references, ...definitions]);
}
function FromInteger(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else if (schema.minimum !== undefined) {
        return schema.minimum;
    }
    else {
        return 0;
    }
}
function FromIntersect(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        // --------------------------------------------------------------
        // Note: The best we can do here is attempt to instance each
        // sub type and apply through object assign. For non-object
        // sub types, we just escape the assignment and just return
        // the value. In the latter case, this is typically going to
        // be a consequence of an illogical intersection.
        // --------------------------------------------------------------
        const value = schema.allOf.reduce((acc, schema) => {
            const next = Visit(schema, references);
            return typeof next === 'object' ? { ...acc, ...next } : next;
        }, {});
        if (!Check(schema, references, value))
            throw new ValueCreateError(schema, 'Intersect produced invalid value. Consider using a default value.');
        return value;
    }
}
function FromIterator(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return (function* () { })();
    }
}
function FromLiteral(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return schema.const;
    }
}
function FromNever(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        throw new ValueCreateError(schema, 'Never types cannot be created. Consider using a default value.');
    }
}
function FromNot(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        throw new ValueCreateError(schema, 'Not types must have a default value');
    }
}
function FromNull(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return null;
    }
}
function FromNumber(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else if (schema.minimum !== undefined) {
        return schema.minimum;
    }
    else {
        return 0;
    }
}
function FromObject(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        const required = new Set(schema.required);
        const Acc = {};
        for (const [key, subschema] of Object.entries(schema.properties)) {
            if (!required.has(key))
                continue;
            Acc[key] = Visit(subschema, references);
        }
        return Acc;
    }
}
function FromPromise(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return Promise.resolve(Visit(schema.item, references));
    }
}
function FromRecord(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return {};
    }
}
function FromRef(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return Visit(Deref(schema, references), references);
    }
}
function FromRegExp(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        throw new ValueCreateError(schema, 'RegExp types cannot be created. Consider using a default value.');
    }
}
function FromString(schema, references) {
    if (schema.pattern !== undefined) {
        if (!HasPropertyKey(schema, 'default')) {
            throw new ValueCreateError(schema, 'String types with patterns must specify a default value');
        }
        else {
            return FromDefault(schema.default);
        }
    }
    else if (schema.format !== undefined) {
        if (!HasPropertyKey(schema, 'default')) {
            throw new ValueCreateError(schema, 'String types with formats must specify a default value');
        }
        else {
            return FromDefault(schema.default);
        }
    }
    else {
        if (HasPropertyKey(schema, 'default')) {
            return FromDefault(schema.default);
        }
        else if (schema.minLength !== undefined) {
            // prettier-ignore
            return Array.from({ length: schema.minLength }).map(() => ' ').join('');
        }
        else {
            return '';
        }
    }
}
function FromSymbol(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else if ('value' in schema) {
        return Symbol.for(schema.value);
    }
    else {
        return Symbol();
    }
}
function FromTemplateLiteral(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    if (!IsTemplateLiteralFinite(schema))
        throw new ValueCreateError(schema, 'Can only create template literals that produce a finite variants. Consider using a default value.');
    const generated = TemplateLiteralGenerate(schema);
    return generated[0];
}
function FromThis(schema, references) {
    if (recursiveDepth++ > recursiveMaxDepth)
        throw new ValueCreateError(schema, 'Cannot create recursive type as it appears possibly infinite. Consider using a default.');
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return Visit(Deref(schema, references), references);
    }
}
function FromTuple(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    if (schema.items === undefined) {
        return [];
    }
    else {
        return Array.from({ length: schema.minItems }).map((_, index) => Visit(schema.items[index], references));
    }
}
function FromUndefined(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return undefined;
    }
}
function FromUnion(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else if (schema.anyOf.length === 0) {
        throw new Error('ValueCreate.Union: Cannot create Union with zero variants');
    }
    else {
        return Visit(schema.anyOf[0], references);
    }
}
function FromUint8Array(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else if (schema.minByteLength !== undefined) {
        return new Uint8Array(schema.minByteLength);
    }
    else {
        return new Uint8Array(0);
    }
}
function FromUnknown(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return {};
    }
}
function FromVoid(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        return void 0;
    }
}
function FromKind(schema, references) {
    if (HasPropertyKey(schema, 'default')) {
        return FromDefault(schema.default);
    }
    else {
        throw new Error('User defined types must specify a default value');
    }
}
function Visit(schema, references) {
    const references_ = Pushref(schema, references);
    const schema_ = schema;
    switch (schema_[Kind]) {
        case 'Any':
            return FromAny(schema_, references_);
        case 'Argument':
            return FromArgument(schema_, references_);
        case 'Array':
            return FromArray(schema_, references_);
        case 'AsyncIterator':
            return FromAsyncIterator(schema_, references_);
        case 'BigInt':
            return FromBigInt(schema_, references_);
        case 'Boolean':
            return FromBoolean(schema_, references_);
        case 'Constructor':
            return FromConstructor(schema_, references_);
        case 'Date':
            return FromDate(schema_, references_);
        case 'Function':
            return FromFunction(schema_, references_);
        case 'Import':
            return FromImport(schema_, references_);
        case 'Integer':
            return FromInteger(schema_, references_);
        case 'Intersect':
            return FromIntersect(schema_, references_);
        case 'Iterator':
            return FromIterator(schema_, references_);
        case 'Literal':
            return FromLiteral(schema_, references_);
        case 'Never':
            return FromNever(schema_, references_);
        case 'Not':
            return FromNot(schema_, references_);
        case 'Null':
            return FromNull(schema_, references_);
        case 'Number':
            return FromNumber(schema_, references_);
        case 'Object':
            return FromObject(schema_, references_);
        case 'Promise':
            return FromPromise(schema_, references_);
        case 'Record':
            return FromRecord(schema_, references_);
        case 'Ref':
            return FromRef(schema_, references_);
        case 'RegExp':
            return FromRegExp(schema_, references_);
        case 'String':
            return FromString(schema_, references_);
        case 'Symbol':
            return FromSymbol(schema_, references_);
        case 'TemplateLiteral':
            return FromTemplateLiteral(schema_, references_);
        case 'This':
            return FromThis(schema_, references_);
        case 'Tuple':
            return FromTuple(schema_, references_);
        case 'Undefined':
            return FromUndefined(schema_, references_);
        case 'Union':
            return FromUnion(schema_, references_);
        case 'Uint8Array':
            return FromUint8Array(schema_, references_);
        case 'Unknown':
            return FromUnknown(schema_, references_);
        case 'Void':
            return FromVoid(schema_, references_);
        default:
            if (!TypeRegistry.Has(schema_[Kind]))
                throw new ValueCreateError(schema_, 'Unknown type');
            return FromKind(schema_, references_);
    }
}
// ------------------------------------------------------------------
// State
// ------------------------------------------------------------------
const recursiveMaxDepth = 512;
let recursiveDepth = 0;
/** Creates a value from the given schema */
export function Create(...args) {
    recursiveDepth = 0;
    return args.length === 2 ? Visit(args[0], args[1]) : Visit(args[0], []);
}
