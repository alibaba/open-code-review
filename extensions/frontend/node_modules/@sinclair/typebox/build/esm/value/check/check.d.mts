import { TypeBoxError } from '../../type/error/index.mjs';
import type { TSchema } from '../../type/schema/index.mjs';
import type { Static } from '../../type/static/index.mjs';
export declare class ValueCheckUnknownTypeError extends TypeBoxError {
    readonly schema: TSchema;
    constructor(schema: TSchema);
}
/** Returns true if the value matches the given type. */
export declare function Check<T extends TSchema>(schema: T, references: TSchema[], value: unknown): value is Static<T>;
/** Returns true if the value matches the given type. */
export declare function Check<T extends TSchema>(schema: T, value: unknown): value is Static<T>;
