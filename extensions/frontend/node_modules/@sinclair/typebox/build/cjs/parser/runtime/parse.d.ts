import * as Types from './types';
/** Parses content using the given Parser */
export declare function Parse<Parser extends Types.IParser>(moduleProperties: Types.IModuleProperties, parser: Parser, code: string, context: unknown): [] | [Types.StaticParser<Parser>, string];
/** Parses content using the given Parser */
export declare function Parse<Parser extends Types.IParser>(moduleProperties: Types.IModuleProperties, parser: Parser, code: string): [] | [Types.StaticParser<Parser>, string];
/** Parses content using the given Parser */
export declare function Parse<Parser extends Types.IParser>(parser: Parser, content: string, context: unknown): [] | [Types.StaticParser<Parser>, string];
/** Parses content using the given Parser */
export declare function Parse<Parser extends Types.IParser>(parser: Parser, content: string): [] | [Types.StaticParser<Parser>, string];
