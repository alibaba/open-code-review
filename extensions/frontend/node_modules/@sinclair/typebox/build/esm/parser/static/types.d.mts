/**
 * `[ACTION]` Inference mapping base type. Used to specify semantic actions for
 * Parser productions. This type is implemented as a higher-kinded type where
 * productions are received on the `input` property with mapping assigned
 * the `output` property. The parsing context is available on the `context`
 * property.
 */
export interface IMapping {
    context: unknown;
    input: unknown;
    output: unknown;
}
/** `[ACTION]` Default inference mapping. */
export interface Identity extends IMapping {
    output: this['input'];
}
/** `[ACTION]` Maps the given argument `T` as the mapping output */
export interface As<T> extends IMapping {
    output: T;
}
/** Base type Parser implemented by all other parsers */
export interface IParser<Mapping extends IMapping = Identity> {
    type: string;
    mapping: Mapping;
}
/** `[Context]` Creates a Context Parser */
export interface Context<Left extends IParser = IParser, Right extends IParser = IParser, Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Context';
    left: Left;
    right: Right;
}
/** `[EBNF]` Creates an Array Parser */
export interface Array<Parser extends IParser = IParser, Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Array';
    parser: Parser;
}
/** `[TERM]` Creates a Const Parser */
export interface Const<Value extends string = string, Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Const';
    value: Value;
}
/** `[TERM]` Creates an Ident Parser. */
export interface Ident<Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Ident';
}
/** `[TERM]` Creates a Number Parser. */
export interface Number<Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Number';
}
/** `[EBNF]` Creates a Optional Parser */
export interface Optional<Parser extends IParser = IParser, Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Optional';
    parser: Parser;
}
/** `[TERM]` Creates a String Parser. Options are an array of permissable quote characters */
export interface String<Options extends string[], Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'String';
    quote: Options;
}
/** `[BNF]` Creates a Tuple Parser */
export interface Tuple<Parsers extends IParser[] = [], Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Tuple';
    parsers: [...Parsers];
}
/** `[BNF]` Creates a Union Parser */
export interface Union<Parsers extends IParser[] = [], Mapping extends IMapping = Identity> extends IParser<Mapping> {
    type: 'Union';
    parsers: [...Parsers];
}
