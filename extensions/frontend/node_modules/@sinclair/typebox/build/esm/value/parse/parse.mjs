import { TypeBoxError } from '../../type/error/index.mjs';
import { TransformDecode, TransformEncode, HasTransform } from '../transform/index.mjs';
import { Assert } from '../assert/index.mjs';
import { Cast } from '../cast/index.mjs';
import { Clean } from '../clean/index.mjs';
import { Clone } from '../clone/index.mjs';
import { Convert } from '../convert/index.mjs';
import { Default } from '../default/index.mjs';
// ------------------------------------------------------------------
// Guards
// ------------------------------------------------------------------
import { IsArray, IsUndefined } from '../guard/index.mjs';
// ------------------------------------------------------------------
// Error
// ------------------------------------------------------------------
export class ParseError extends TypeBoxError {
    constructor(message) {
        super(message);
    }
}
// prettier-ignore
export var ParseRegistry;
(function (ParseRegistry) {
    const registry = new Map([
        ['Assert', (type, references, value) => { Assert(type, references, value); return value; }],
        ['Cast', (type, references, value) => Cast(type, references, value)],
        ['Clean', (type, references, value) => Clean(type, references, value)],
        ['Clone', (_type, _references, value) => Clone(value)],
        ['Convert', (type, references, value) => Convert(type, references, value)],
        ['Decode', (type, references, value) => (HasTransform(type, references) ? TransformDecode(type, references, value) : value)],
        ['Default', (type, references, value) => Default(type, references, value)],
        ['Encode', (type, references, value) => (HasTransform(type, references) ? TransformEncode(type, references, value) : value)],
    ]);
    // Deletes an entry from the registry
    function Delete(key) {
        registry.delete(key);
    }
    ParseRegistry.Delete = Delete;
    // Sets an entry in the registry
    function Set(key, callback) {
        registry.set(key, callback);
    }
    ParseRegistry.Set = Set;
    // Gets an entry in the registry
    function Get(key) {
        return registry.get(key);
    }
    ParseRegistry.Get = Get;
})(ParseRegistry || (ParseRegistry = {}));
// ------------------------------------------------------------------
// Default Parse Pipeline
// ------------------------------------------------------------------
// prettier-ignore
export const ParseDefault = [
    'Clone',
    'Clean',
    'Default',
    'Convert',
    'Assert',
    'Decode'
];
// ------------------------------------------------------------------
// ParseValue
// ------------------------------------------------------------------
function ParseValue(operations, type, references, value) {
    return operations.reduce((value, operationKey) => {
        const operation = ParseRegistry.Get(operationKey);
        if (IsUndefined(operation))
            throw new ParseError(`Unable to find Parse operation '${operationKey}'`);
        return operation(type, references, value);
    }, value);
}
/** Parses a value */
export function Parse(...args) {
    // prettier-ignore
    const [operations, schema, references, value] = (args.length === 4 ? [args[0], args[1], args[2], args[3]] :
        args.length === 3 ? IsArray(args[0]) ? [args[0], args[1], [], args[2]] : [ParseDefault, args[0], args[1], args[2]] :
            args.length === 2 ? [ParseDefault, args[0], [], args[1]] :
                (() => { throw new ParseError('Invalid Arguments'); })());
    return ParseValue(operations, schema, references, value);
}
