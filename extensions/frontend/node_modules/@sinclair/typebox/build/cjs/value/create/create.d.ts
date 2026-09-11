import { TypeBoxError } from '../../type/error/index';
import type { TSchema } from '../../type/schema/index';
import type { Static } from '../../type/static/index';
export declare class ValueCreateError extends TypeBoxError {
    readonly schema: TSchema;
    constructor(schema: TSchema, message: string);
}
/** Creates a value from the given schema and references */
export declare function Create<T extends TSchema>(schema: T, references: TSchema[]): Static<T>;
/** Creates a value from the given schema */
export declare function Create<T extends TSchema>(schema: T): Static<T>;
