import { TSchema, SchemaOptions } from '../schema/index.mjs';
import { Kind } from '../symbols/index.mjs';
export type StringFormatOption = 'date-time' | 'time' | 'date' | 'email' | 'idn-email' | 'hostname' | 'idn-hostname' | 'ipv4' | 'ipv6' | 'uri' | 'uri-reference' | 'iri' | 'uuid' | 'iri-reference' | 'uri-template' | 'json-pointer' | 'relative-json-pointer' | 'regex' | ({} & string);
export type StringContentEncodingOption = '7bit' | '8bit' | 'binary' | 'quoted-printable' | 'base64' | ({} & string);
export interface StringOptions extends SchemaOptions {
    /** The maximum string length */
    maxLength?: number;
    /** The minimum string length */
    minLength?: number;
    /** A regular expression pattern this string should match */
    pattern?: string;
    /** A format this string should match */
    format?: StringFormatOption;
    /** The content encoding for this string */
    contentEncoding?: StringContentEncodingOption;
    /** The content media type for this string */
    contentMediaType?: string;
}
export interface TString extends TSchema, StringOptions {
    [Kind]: 'String';
    static: string;
    type: 'string';
}
/** `[Json]` Creates a String type */
export declare function String(options?: StringOptions): TString;
