import * as Tokens from './token';
import * as Types from './types';
type ContextParser<Left extends Types.IParser, Right extends Types.IParser, Code extends string, Context extends unknown> = (Parse<Left, Code, Context> extends [infer Context extends unknown, infer Rest extends string] ? Parse<Right, Rest, Context> : []);
type ArrayParser<Parser extends Types.IParser, Code extends string, Context extends unknown, Result extends unknown[] = []> = (Parse<Parser, Code, Context> extends [infer Value1 extends unknown, infer Rest extends string] ? ArrayParser<Parser, Rest, Context, [...Result, Value1]> : [Result, Code]);
type ConstParser<Value extends string, Code extends string, _Context extends unknown> = (Tokens.Const<Value, Code> extends [infer Match extends Value, infer Rest extends string] ? [Match, Rest] : []);
type IdentParser<Code extends string, _Context extends unknown> = (Tokens.Ident<Code> extends [infer Match extends string, infer Rest extends string] ? [Match, Rest] : []);
type NumberParser<Code extends string, _Context extends unknown> = (Tokens.Number<Code> extends [infer Match extends string, infer Rest extends string] ? [Match, Rest] : []);
type OptionalParser<Parser extends Types.IParser, Code extends string, Context extends unknown> = (Parse<Parser, Code, Context> extends [infer Value extends unknown, infer Rest extends string] ? [[Value], Rest] : [[], Code]);
type StringParser<Options extends string[], Code extends string, _Context extends unknown> = (Tokens.String<Options, Code> extends [infer Match extends string, infer Rest extends string] ? [Match, Rest] : []);
type TupleParser<Parsers extends Types.IParser[], Code extends string, Context extends unknown, Result extends unknown[] = []> = (Parsers extends [infer Left extends Types.IParser, ...infer Right extends Types.IParser[]] ? Parse<Left, Code, Context> extends [infer Value extends unknown, infer Rest extends string] ? TupleParser<Right, Rest, Context, [...Result, Value]> : [] : [Result, Code]);
type UnionParser<Parsers extends Types.IParser[], Code extends string, Context extends unknown> = (Parsers extends [infer Left extends Types.IParser, ...infer Right extends Types.IParser[]] ? Parse<Left, Code, Context> extends [infer Value extends unknown, infer Rest extends string] ? [Value, Rest] : UnionParser<Right, Code, Context> : []);
type ParseCode<Type extends Types.IParser, Code extends string, Context extends unknown = unknown> = (Type extends Types.Context<infer Left extends Types.IParser, infer Right extends Types.IParser> ? ContextParser<Left, Right, Code, Context> : Type extends Types.Array<infer Parser extends Types.IParser> ? ArrayParser<Parser, Code, Context> : Type extends Types.Const<infer Value extends string> ? ConstParser<Value, Code, Context> : Type extends Types.Ident ? IdentParser<Code, Context> : Type extends Types.Number ? NumberParser<Code, Context> : Type extends Types.Optional<infer Parser extends Types.IParser> ? OptionalParser<Parser, Code, Context> : Type extends Types.String<infer Options extends string[]> ? StringParser<Options, Code, Context> : Type extends Types.Tuple<infer Parsers extends Types.IParser[]> ? TupleParser<Parsers, Code, Context> : Type extends Types.Union<infer Parsers extends Types.IParser[]> ? UnionParser<Parsers, Code, Context> : [
]);
type ParseMapping<Parser extends Types.IParser, Result extends unknown, Context extends unknown = unknown> = ((Parser['mapping'] & {
    input: Result;
    context: Context;
})['output']);
/** Parses code with the given parser */
export type Parse<Type extends Types.IParser, Code extends string, Context extends unknown = unknown> = (ParseCode<Type, Code, Context> extends [infer L extends unknown, infer R extends string] ? [ParseMapping<Type, L, Context>, R] : []);
export {};
