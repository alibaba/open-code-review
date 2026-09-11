import { TypeBoxError } from '../../type/error/index';
import type { TSchema } from '../../type/schema/index';
import type { Static } from '../../type/static/index';
export declare class ValueCastError extends TypeBoxError {
    readonly schema: TSchema;
    constructor(schema: TSchema, message: string);
}
/** Casts a value into a given type and references. The return value will retain as much information of the original value as possible. */
export declare function Cast<T extends TSchema>(schema: T, references: TSchema[], value: unknown): Static<T>;
/** Casts a value into a given type. The return value will retain as much information of the original value as possible. */
export declare function Cast<T extends TSchema>(schema: T, value: unknown): Static<T>;
