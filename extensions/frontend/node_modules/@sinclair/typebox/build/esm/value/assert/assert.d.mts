import { ValueErrorIterator, ValueError } from '../../errors/index.mjs';
import { TypeBoxError } from '../../type/error/error.mjs';
import { TSchema } from '../../type/schema/index.mjs';
import { Static } from '../../type/static/index.mjs';
export declare class AssertError extends TypeBoxError {
    #private;
    error: ValueError | undefined;
    constructor(iterator: ValueErrorIterator);
    /** Returns an iterator for each error in this value. */
    Errors(): ValueErrorIterator;
}
/** Asserts a value matches the given type or throws an `AssertError` if invalid */
export declare function Assert<T extends TSchema>(schema: T, references: TSchema[], value: unknown): asserts value is Static<T>;
/** Asserts a value matches the given type or throws an `AssertError` if invalid */
export declare function Assert<T extends TSchema>(schema: T, value: unknown): asserts value is Static<T>;
