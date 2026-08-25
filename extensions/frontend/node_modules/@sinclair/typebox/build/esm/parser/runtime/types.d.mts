export type IModuleProperties = Record<PropertyKey, IParser>;
/** Force output static type evaluation for Arrays */
export type StaticEnsure<T> = T extends infer R ? R : never;
/** Infers the Output Parameter for a Parser */
export type StaticParser<Parser extends IParser> = Parser extends IParser<infer Output extends unknown> ? Output : unknown;
export type IMapping<Input extends unknown = any, Output extends unknown = unknown> = (input: Input, context: any) => Output;
/** Maps input to output. This is the default Mapping */
export declare const Identity: (value: unknown) => unknown;
/** Maps the output as the given parameter T */
export declare const As: <T>(mapping: T) => ((value: unknown) => T);
export interface IParser<Output extends unknown = unknown> {
    type: string;
    mapping: IMapping<any, Output>;
}
export type ContextParameter<_Left extends IParser, Right extends IParser> = (StaticParser<Right>);
export interface IContext<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Context';
    left: IParser;
    right: IParser;
}
/** `[Context]` Creates a Context Parser */
export declare function Context<Left extends IParser, Right extends IParser, Mapping extends IMapping = IMapping<ContextParameter<Left, Right>>>(left: Left, right: Right, mapping: Mapping): IContext<ReturnType<Mapping>>;
/** `[Context]` Creates a Context Parser */
export declare function Context<Left extends IParser, Right extends IParser>(left: Left, right: Right): IContext<ContextParameter<Left, Right>>;
export type ArrayParameter<Parser extends IParser> = StaticEnsure<StaticParser<Parser>[]>;
export interface IArray<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Array';
    parser: IParser;
}
/** `[EBNF]` Creates an Array Parser */
export declare function Array<Parser extends IParser, Mapping extends IMapping = IMapping<ArrayParameter<Parser>>>(parser: Parser, mapping: Mapping): IArray<ReturnType<Mapping>>;
/** `[EBNF]` Creates an Array Parser */
export declare function Array<Parser extends IParser>(parser: Parser): IArray<ArrayParameter<Parser>>;
export interface IConst<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Const';
    value: string;
}
/** `[TERM]` Creates a Const Parser */
export declare function Const<Value extends string, Mapping extends IMapping<Value>>(value: Value, mapping: Mapping): IConst<ReturnType<Mapping>>;
/** `[TERM]` Creates a Const Parser */
export declare function Const<Value extends string>(value: Value): IConst<Value>;
export interface IRef<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Ref';
    ref: string;
}
/** `[BNF]` Creates a Ref Parser. This Parser can only be used in the context of a Module */
export declare function Ref<Type extends unknown, Mapping extends IMapping<Type>>(ref: string, mapping: Mapping): IRef<ReturnType<Mapping>>;
/** `[BNF]` Creates a Ref Parser. This Parser can only be used in the context of a Module */
export declare function Ref<Type extends unknown>(ref: string): IRef<Type>;
export interface IString<Output extends unknown = unknown> extends IParser<Output> {
    type: 'String';
    options: string[];
}
/** `[TERM]` Creates a String Parser. Options are an array of permissable quote characters */
export declare function String<Mapping extends IMapping<string>>(options: string[], mapping: Mapping): IString<ReturnType<Mapping>>;
/** `[TERM]` Creates a String Parser. Options are an array of permissable quote characters */
export declare function String(options: string[]): IString<string>;
export interface IIdent<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Ident';
}
/** `[TERM]` Creates an Ident Parser where Ident matches any valid JavaScript identifier */
export declare function Ident<Mapping extends IMapping<string>>(mapping: Mapping): IIdent<ReturnType<Mapping>>;
/** `[TERM]` Creates an Ident Parser where Ident matches any valid JavaScript identifier */
export declare function Ident(): IIdent<string>;
export interface INumber<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Number';
}
/** `[TERM]` Creates an Number Parser */
export declare function Number<Mapping extends IMapping<string>>(mapping: Mapping): INumber<ReturnType<Mapping>>;
/** `[TERM]` Creates an Number Parser */
export declare function Number(): INumber<string>;
export type OptionalParameter<Parser extends IParser, Result extends unknown = [StaticParser<Parser>] | []> = (Result);
export interface IOptional<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Optional';
    parser: IParser;
}
/** `[EBNF]` Creates an Optional Parser */
export declare function Optional<Parser extends IParser, Mapping extends IMapping = IMapping<OptionalParameter<Parser>>>(parser: Parser, mapping: Mapping): IOptional<ReturnType<Mapping>>;
/** `[EBNF]` Creates an Optional Parser */
export declare function Optional<Parser extends IParser>(parser: Parser): IOptional<OptionalParameter<Parser>>;
export type TupleParameter<Parsers extends IParser[], Result extends unknown[] = []> = StaticEnsure<Parsers extends [infer Left extends IParser, ...infer Right extends IParser[]] ? TupleParameter<Right, [...Result, StaticEnsure<StaticParser<Left>>]> : Result>;
export interface ITuple<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Tuple';
    parsers: IParser[];
}
/** `[BNF]` Creates a Tuple Parser */
export declare function Tuple<Parsers extends IParser[], Mapping extends IMapping = IMapping<TupleParameter<Parsers>>>(parsers: [...Parsers], mapping: Mapping): ITuple<ReturnType<Mapping>>;
/** `[BNF]` Creates a Tuple Parser */
export declare function Tuple<Parsers extends IParser[]>(parsers: [...Parsers]): ITuple<TupleParameter<Parsers>>;
export type UnionParameter<Parsers extends IParser[], Result extends unknown = never> = StaticEnsure<Parsers extends [infer Left extends IParser, ...infer Right extends IParser[]] ? UnionParameter<Right, Result | StaticParser<Left>> : Result>;
export interface IUnion<Output extends unknown = unknown> extends IParser<Output> {
    type: 'Union';
    parsers: IParser[];
}
/** `[BNF]` Creates a Union parser */
export declare function Union<Parsers extends IParser[], Mapping extends IMapping = IMapping<UnionParameter<Parsers>>>(parsers: [...Parsers], mapping: Mapping): IUnion<ReturnType<Mapping>>;
/** `[BNF]` Creates a Union parser */
export declare function Union<Parsers extends IParser[]>(parsers: [...Parsers]): IUnion<UnionParameter<Parsers>>;
