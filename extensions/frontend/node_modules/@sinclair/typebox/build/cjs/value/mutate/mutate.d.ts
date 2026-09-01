import { TypeBoxError } from '../../type/error/index';
export declare class ValueMutateError extends TypeBoxError {
    constructor(message: string);
}
export type Mutable = {
    [key: string]: unknown;
} | unknown[];
/** `[Mutable]` Performs a deep mutable value assignment while retaining internal references */
export declare function Mutate(current: Mutable, next: Mutable): void;
