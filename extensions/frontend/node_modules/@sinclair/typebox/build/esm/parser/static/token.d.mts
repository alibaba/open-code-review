declare namespace Chars {
    type Empty = '';
    type Space = ' ';
    type Newline = '\n';
    type Dot = '.';
    type Hyphen = '-';
    type Digit = [
        '0',
        '1',
        '2',
        '3',
        '4',
        '5',
        '6',
        '7',
        '8',
        '9'
    ];
    type Alpha = [
        'a',
        'b',
        'c',
        'd',
        'e',
        'f',
        'g',
        'h',
        'i',
        'j',
        'k',
        'l',
        'm',
        'n',
        'o',
        'p',
        'q',
        'r',
        's',
        't',
        'u',
        'v',
        'w',
        'x',
        'y',
        'z',
        'A',
        'B',
        'C',
        'D',
        'E',
        'F',
        'G',
        'H',
        'I',
        'J',
        'K',
        'L',
        'M',
        'N',
        'O',
        'P',
        'Q',
        'R',
        'S',
        'T',
        'U',
        'V',
        'W',
        'X',
        'Y',
        'Z'
    ];
}
declare namespace Trim {
    type W4 = `${W3}${W3}`;
    type W3 = `${W2}${W2}`;
    type W2 = `${W1}${W1}`;
    type W1 = `${W0}${W0}`;
    type W0 = ` `;
    /** Trims whitespace only */
    export type TrimWhitespace<Code extends string> = (Code extends `${W4}${infer Rest extends string}` ? TrimWhitespace<Rest> : Code extends `${W3}${infer Rest extends string}` ? TrimWhitespace<Rest> : Code extends `${W1}${infer Rest extends string}` ? TrimWhitespace<Rest> : Code extends `${W0}${infer Rest extends string}` ? TrimWhitespace<Rest> : Code);
    /** Trims Whitespace and Newline */
    export type TrimAll<Code extends string> = (Code extends `${W4}${infer Rest extends string}` ? TrimAll<Rest> : Code extends `${W3}${infer Rest extends string}` ? TrimAll<Rest> : Code extends `${W1}${infer Rest extends string}` ? TrimAll<Rest> : Code extends `${W0}${infer Rest extends string}` ? TrimAll<Rest> : Code extends `${Chars.Newline}${infer Rest extends string}` ? TrimAll<Rest> : Code);
    export {};
}
/** Scans for the next match union */
type NextUnion<Variants extends string[], Code extends string> = (Variants extends [infer Variant extends string, ...infer Rest1 extends string[]] ? NextConst<Variant, Code> extends [infer Match extends string, infer Rest2 extends string] ? [Match, Rest2] : NextUnion<Rest1, Code> : []);
type NextConst<Value extends string, Code extends string> = (Code extends `${Value}${infer Rest extends string}` ? [Value, Rest] : []);
/** Scans for the next constant value */
export type Const<Value extends string, Code extends string> = (Value extends '' ? ['', Code] : Value extends `${infer First extends string}${string}` ? (First extends Chars.Newline ? NextConst<Value, Trim.TrimWhitespace<Code>> : First extends Chars.Space ? NextConst<Value, Code> : NextConst<Value, Trim.TrimAll<Code>>) : never);
type NextNumberNegate<Code extends string> = (Code extends `${Chars.Hyphen}${infer Rest extends string}` ? [Chars.Hyphen, Rest] : [Chars.Empty, Code]);
type NextNumberZeroCheck<Code extends string> = (Code extends `0${infer Rest}` ? NextUnion<Chars.Digit, Rest> extends [string, string] ? false : true : true);
type NextNumberScan<Code extends string, HasDecimal extends boolean = false, Result extends string = Chars.Empty> = (NextUnion<[...Chars.Digit, Chars.Dot], Code> extends [infer Char extends string, infer Rest extends string] ? Char extends Chars.Dot ? HasDecimal extends false ? NextNumberScan<Rest, true, `${Result}${Char}`> : [Result, `.${Rest}`] : NextNumberScan<Rest, HasDecimal, `${Result}${Char}`> : [Result, Code]);
export type NextNumber<Code extends string> = (NextNumberNegate<Code> extends [infer Negate extends string, infer Rest extends string] ? NextNumberZeroCheck<Rest> extends true ? NextNumberScan<Rest> extends [infer Number extends string, infer Rest2 extends string] ? Number extends Chars.Empty ? [] : [`${Negate}${Number}`, Rest2] : [] : [] : []);
/** Scans for the next literal number */
export type Number<Code extends string> = NextNumber<Trim.TrimAll<Code>>;
type NextStringQuote<Options extends string[], Code extends string> = NextUnion<Options, Code>;
type NextStringBody<Code extends string, Quote extends string, Result extends string = Chars.Empty> = (Code extends `${infer Char extends string}${infer Rest extends string}` ? Char extends Quote ? [Result, Rest] : NextStringBody<Rest, Quote, `${Result}${Char}`> : []);
type NextString<Options extends string[], Code extends string> = (NextStringQuote<Options, Code> extends [infer Quote extends string, infer Rest extends string] ? NextStringBody<Rest, Quote> extends [infer String extends string, infer Rest extends string] ? [String, Rest] : [] : []);
/** Scans for the next literal string */
export type String<Options extends string[], Code extends string> = NextString<Options, Trim.TrimAll<Code>>;
type IdentLeft = [...Chars.Alpha, '_', '$'];
type IdentRight = [...Chars.Digit, ...IdentLeft];
type NextIdentScan<Code extends string, Result extends string = Chars.Empty> = (NextUnion<IdentRight, Code> extends [infer Char extends string, infer Rest extends string] ? NextIdentScan<Rest, `${Result}${Char}`> : [Result, Code]);
type NextIdent<Code extends string> = (NextUnion<IdentLeft, Code> extends [infer Left extends string, infer Rest1 extends string] ? NextIdentScan<Rest1> extends [infer Right extends string, infer Rest2 extends string] ? [`${Left}${Right}`, Rest2] : [] : []);
/** Scans for the next Ident */
export type Ident<Code extends string> = NextIdent<Trim.TrimAll<Code>>;
export {};
