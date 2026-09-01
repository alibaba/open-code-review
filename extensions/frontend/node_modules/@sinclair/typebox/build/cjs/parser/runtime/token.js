"use strict";

Object.defineProperty(exports, "__esModule", { value: true });
exports.Const = Const;
exports.Ident = Ident;
exports.Number = Number;
exports.String = String;
// ------------------------------------------------------------------
// Chars
// ------------------------------------------------------------------
// prettier-ignore
var Chars;
(function (Chars) {
    /** Returns true if the char code is a whitespace */
    function IsWhitespace(value) {
        return value === 32;
    }
    Chars.IsWhitespace = IsWhitespace;
    /** Returns true if the char code is a newline */
    function IsNewline(value) {
        return value === 10;
    }
    Chars.IsNewline = IsNewline;
    /** Returns true if the char code is a alpha  */
    function IsAlpha(value) {
        return ((value >= 65 && value <= 90) || // A-Z 
            (value >= 97 && value <= 122) // a-z
        );
    }
    Chars.IsAlpha = IsAlpha;
    /** Returns true if the char code is zero */
    function IsZero(value) {
        return value === 48;
    }
    Chars.IsZero = IsZero;
    /** Returns true if the char code is non-zero */
    function IsNonZero(value) {
        return value >= 49 && value <= 57;
    }
    Chars.IsNonZero = IsNonZero;
    /** Returns true if the char code is a digit */
    function IsDigit(value) {
        return (IsNonZero(value) ||
            IsZero(value));
    }
    Chars.IsDigit = IsDigit;
    /** Returns true if the char code is a dot */
    function IsDot(value) {
        return value === 46;
    }
    Chars.IsDot = IsDot;
    /** Returns true if this char code is a underscore */
    function IsUnderscore(value) {
        return value === 95;
    }
    Chars.IsUnderscore = IsUnderscore;
    /** Returns true if this char code is a dollar sign */
    function IsDollarSign(value) {
        return value === 36;
    }
    Chars.IsDollarSign = IsDollarSign;
})(Chars || (Chars = {}));
// ------------------------------------------------------------------
// Trim
// ------------------------------------------------------------------
// prettier-ignore
var Trim;
(function (Trim) {
    /** Trims Whitespace and retains Newline, Tabspaces, etc. */
    function TrimWhitespaceOnly(code) {
        for (let i = 0; i < code.length; i++) {
            if (Chars.IsWhitespace(code.charCodeAt(i)))
                continue;
            return code.slice(i);
        }
        return code;
    }
    Trim.TrimWhitespaceOnly = TrimWhitespaceOnly;
    /** Trims Whitespace including Newline, Tabspaces, etc. */
    function TrimAll(code) {
        return code.trimStart();
    }
    Trim.TrimAll = TrimAll;
})(Trim || (Trim = {}));
// ------------------------------------------------------------------
// Const
// ------------------------------------------------------------------
/** Checks the value matches the next string  */
// prettier-ignore
function NextTokenCheck(value, code) {
    if (value.length > code.length)
        return false;
    for (let i = 0; i < value.length; i++) {
        if (value.charCodeAt(i) !== code.charCodeAt(i))
            return false;
    }
    return true;
}
/** Gets the next constant string value or empty if no match */
// prettier-ignore
function NextConst(value, code) {
    return NextTokenCheck(value, code)
        ? [code.slice(0, value.length), code.slice(value.length)]
        : [];
}
/** Takes the next constant string value skipping any whitespace */
// prettier-ignore
function Const(value, code) {
    if (value.length === 0)
        return ['', code];
    const char_0 = value.charCodeAt(0);
    return (Chars.IsNewline(char_0) ? NextConst(value, Trim.TrimWhitespaceOnly(code)) :
        Chars.IsWhitespace(char_0) ? NextConst(value, code) :
            NextConst(value, Trim.TrimAll(code)));
}
// ------------------------------------------------------------------
// Ident
// ------------------------------------------------------------------
// prettier-ignore
function IdentIsFirst(char) {
    return (Chars.IsAlpha(char) ||
        Chars.IsDollarSign(char) ||
        Chars.IsUnderscore(char));
}
// prettier-ignore
function IdentIsRest(char) {
    return (Chars.IsAlpha(char) ||
        Chars.IsDigit(char) ||
        Chars.IsDollarSign(char) ||
        Chars.IsUnderscore(char));
}
// prettier-ignore
function NextIdent(code) {
    if (!IdentIsFirst(code.charCodeAt(0)))
        return [];
    for (let i = 1; i < code.length; i++) {
        const char = code.charCodeAt(i);
        if (IdentIsRest(char))
            continue;
        const slice = code.slice(0, i);
        const rest = code.slice(i);
        return [slice, rest];
    }
    return [code, ''];
}
/** Scans for the next Ident token */
// prettier-ignore
function Ident(code) {
    return NextIdent(Trim.TrimAll(code));
}
// ------------------------------------------------------------------
// Number
// ------------------------------------------------------------------
/** Checks that the next number is not a leading zero */
// prettier-ignore
function NumberLeadingZeroCheck(code, index) {
    const char_0 = code.charCodeAt(index + 0);
    const char_1 = code.charCodeAt(index + 1);
    return ((
    // 1-9
    Chars.IsNonZero(char_0)) || (
    // 0
    Chars.IsZero(char_0) &&
        !Chars.IsDigit(char_1)) || (
    // 0.
    Chars.IsZero(char_0) &&
        Chars.IsDot(char_1)) || (
    // .0
    Chars.IsDot(char_0) &&
        Chars.IsDigit(char_1)));
}
/** Gets the next number token */
// prettier-ignore
function NextNumber(code) {
    const negated = code.charAt(0) === '-';
    const index = negated ? 1 : 0;
    if (!NumberLeadingZeroCheck(code, index)) {
        return [];
    }
    const dash = negated ? '-' : '';
    let hasDot = false;
    for (let i = index; i < code.length; i++) {
        const char_i = code.charCodeAt(i);
        if (Chars.IsDigit(char_i)) {
            continue;
        }
        if (Chars.IsDot(char_i)) {
            if (hasDot) {
                const slice = code.slice(index, i);
                const rest = code.slice(i);
                return [`${dash}${slice}`, rest];
            }
            hasDot = true;
            continue;
        }
        const slice = code.slice(index, i);
        const rest = code.slice(i);
        return [`${dash}${slice}`, rest];
    }
    return [code, ''];
}
/** Scans for the next number token */
// prettier-ignore
function Number(code) {
    return NextNumber(Trim.TrimAll(code));
}
// ------------------------------------------------------------------
// String
// ------------------------------------------------------------------
// prettier-ignore
function NextString(options, code) {
    const first = code.charAt(0);
    if (!options.includes(first))
        return [];
    const quote = first;
    for (let i = 1; i < code.length; i++) {
        const char = code.charAt(i);
        if (char === quote) {
            const slice = code.slice(1, i);
            const rest = code.slice(i + 1);
            return [slice, rest];
        }
    }
    return [];
}
/** Scans the next Literal String value */
// prettier-ignore
function String(options, code) {
    return NextString(options, Trim.TrimAll(code));
}
