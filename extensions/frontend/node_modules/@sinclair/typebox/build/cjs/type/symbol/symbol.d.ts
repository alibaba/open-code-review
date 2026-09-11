import { TSchema, SchemaOptions } from '../schema/index';
import { Kind } from '../symbols/index';
export type TSymbolValue = string | number | undefined;
export interface TSymbol extends TSchema, SchemaOptions {
    [Kind]: 'Symbol';
    static: symbol;
    type: 'symbol';
}
/** `[JavaScript]` Creates a Symbol type */
export declare function Symbol(options?: SchemaOptions): TSymbol;
