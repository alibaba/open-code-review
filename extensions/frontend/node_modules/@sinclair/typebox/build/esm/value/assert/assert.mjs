var __classPrivateFieldSet = (this && this.__classPrivateFieldSet) || function (receiver, state, value, kind, f) {
    if (kind === "m") throw new TypeError("Private method is not writable");
    if (kind === "a" && !f) throw new TypeError("Private accessor was defined without a setter");
    if (typeof state === "function" ? receiver !== state || !f : !state.has(receiver)) throw new TypeError("Cannot write private member to an object whose class did not declare it");
    return (kind === "a" ? f.call(receiver, value) : f ? f.value = value : state.set(receiver, value)), value;
};
var __classPrivateFieldGet = (this && this.__classPrivateFieldGet) || function (receiver, state, kind, f) {
    if (kind === "a" && !f) throw new TypeError("Private accessor was defined without a getter");
    if (typeof state === "function" ? receiver !== state || !f : !state.has(receiver)) throw new TypeError("Cannot read private member from an object whose class did not declare it");
    return kind === "m" ? f : kind === "a" ? f.call(receiver) : f ? f.value : state.get(receiver);
};
var _AssertError_instances, _AssertError_iterator, _AssertError_Iterator;
import { Errors, ValueErrorIterator } from '../../errors/index.mjs';
import { TypeBoxError } from '../../type/error/error.mjs';
import { Check } from '../check/check.mjs';
// ------------------------------------------------------------------
// AssertError
// ------------------------------------------------------------------
export class AssertError extends TypeBoxError {
    constructor(iterator) {
        const error = iterator.First();
        super(error === undefined ? 'Invalid Value' : error.message);
        _AssertError_instances.add(this);
        _AssertError_iterator.set(this, void 0);
        __classPrivateFieldSet(this, _AssertError_iterator, iterator, "f");
        this.error = error;
    }
    /** Returns an iterator for each error in this value. */
    Errors() {
        return new ValueErrorIterator(__classPrivateFieldGet(this, _AssertError_instances, "m", _AssertError_Iterator).call(this));
    }
}
_AssertError_iterator = new WeakMap(), _AssertError_instances = new WeakSet(), _AssertError_Iterator = function* _AssertError_Iterator() {
    if (this.error)
        yield this.error;
    yield* __classPrivateFieldGet(this, _AssertError_iterator, "f");
};
// ------------------------------------------------------------------
// AssertValue
// ------------------------------------------------------------------
function AssertValue(schema, references, value) {
    if (Check(schema, references, value))
        return;
    throw new AssertError(Errors(schema, references, value));
}
/** Asserts a value matches the given type or throws an `AssertError` if invalid */
export function Assert(...args) {
    return args.length === 3 ? AssertValue(args[0], args[1], args[2]) : AssertValue(args[0], [], args[1]);
}
