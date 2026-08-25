import { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export type TSymbolValue = string | number | undefined;
export interface TSymbol extends TSchema, SchemaOptions {
    [Kind]: 'Symbol';
    static: symbol;
    type: 'symbol';
}
/** `[JavaScript]` Creates a Symbol type */
export declare function Symbol(options?: SchemaOptions): TSymbol;
