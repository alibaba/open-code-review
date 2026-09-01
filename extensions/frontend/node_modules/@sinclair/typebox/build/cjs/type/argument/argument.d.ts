import type { TSchema } from '../schema/index';
import { Kind } from '../symbols/index';
export interface TArgument<Index extends number = number> extends TSchema {
    [Kind]: 'Argument';
    static: unknown;
    index: Index;
}
/** `[JavaScript]` Creates an Argument Type. */
export declare function Argument<Index extends number>(index: Index): TArgument<Index>;
