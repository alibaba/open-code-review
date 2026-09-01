import type { TSchema } from '../../type/schema/index';
import type { TRef } from '../../type/ref/index';
import type { TThis } from '../../type/recursive/index';
import { TypeBoxError } from '../../type/error/index';
export declare class TypeDereferenceError extends TypeBoxError {
    readonly schema: TRef | TThis;
    constructor(schema: TRef | TThis);
}
/** `[Internal]` Pushes a schema onto references if the schema has an $id and does not exist on references */
export declare function Pushref(schema: TSchema, references: TSchema[]): TSchema[];
/** `[Internal]` Dereferences a schema from the references array or throws if not found */
export declare function Deref(schema: TSchema, references: TSchema[]): TSchema;
