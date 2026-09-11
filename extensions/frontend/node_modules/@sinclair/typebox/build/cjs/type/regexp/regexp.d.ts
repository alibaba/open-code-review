import type { SchemaOptions } from '../schema/index';
import type { TSchema } from '../schema/index';
import { Kind } from '../symbols/index';
export interface RegExpOptions extends SchemaOptions {
    /** The maximum length of the string */
    maxLength?: number;
    /** The minimum length of the string */
    minLength?: number;
}
export interface TRegExp extends TSchema {
    [Kind]: 'RegExp';
    static: `${string}`;
    type: 'RegExp';
    source: string;
    flags: string;
}
/** `[JavaScript]` Creates a RegExp type */
export declare function RegExp(pattern: string, options?: RegExpOptions): TRegExp;
/** `[JavaScript]` Creates a RegExp type */
export declare function RegExp(regex: RegExp, options?: RegExpOptions): TRegExp;
