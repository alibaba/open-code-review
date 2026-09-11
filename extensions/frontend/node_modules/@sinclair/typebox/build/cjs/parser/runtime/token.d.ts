/** Takes the next constant string value skipping any whitespace */
export declare function Const(value: string, code: string): [] | [string, string];
/** Scans for the next Ident token */
export declare function Ident(code: string): [] | [string, string];
/** Scans for the next number token */
export declare function Number(code: string): [] | [string, string];
/** Scans the next Literal String value */
export declare function String(options: string[], code: string): [] | [string, string];
