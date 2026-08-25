"use strict";

var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.Parse = Parse;
const Guard = __importStar(require("./guard"));
const Token = __importStar(require("./token"));
// ------------------------------------------------------------------
// Context
// ------------------------------------------------------------------
function ParseContext(moduleProperties, left, right, code, context) {
    const result = ParseParser(moduleProperties, left, code, context);
    return result.length === 2 ? ParseParser(moduleProperties, right, result[1], result[0]) : [];
}
// ------------------------------------------------------------------
// Array
// ------------------------------------------------------------------
function ParseArray(moduleProperties, parser, code, context) {
    const buffer = [];
    let rest = code;
    while (rest.length > 0) {
        const result = ParseParser(moduleProperties, parser, rest, context);
        if (result.length === 0)
            return [buffer, rest];
        buffer.push(result[0]);
        rest = result[1];
    }
    return [buffer, rest];
}
// ------------------------------------------------------------------
// Const
// ------------------------------------------------------------------
function ParseConst(value, code, context) {
    return Token.Const(value, code);
}
// ------------------------------------------------------------------
// Ident
// ------------------------------------------------------------------
function ParseIdent(code, _context) {
    return Token.Ident(code);
}
// ------------------------------------------------------------------
// Number
// ------------------------------------------------------------------
// prettier-ignore
function ParseNumber(code, _context) {
    return Token.Number(code);
}
// ------------------------------------------------------------------
// Optional
// ------------------------------------------------------------------
function ParseOptional(moduleProperties, parser, code, context) {
    const result = ParseParser(moduleProperties, parser, code, context);
    return (result.length === 2 ? [[result[0]], result[1]] : [[], code]);
}
// ------------------------------------------------------------------
// Ref
// ------------------------------------------------------------------
function ParseRef(moduleProperties, ref, code, context) {
    const parser = moduleProperties[ref];
    if (!Guard.IsParser(parser))
        throw Error(`Cannot dereference Parser '${ref}'`);
    return ParseParser(moduleProperties, parser, code, context);
}
// ------------------------------------------------------------------
// String
// ------------------------------------------------------------------
// prettier-ignore
function ParseString(options, code, _context) {
    return Token.String(options, code);
}
// ------------------------------------------------------------------
// Tuple
// ------------------------------------------------------------------
function ParseTuple(moduleProperties, parsers, code, context) {
    const buffer = [];
    let rest = code;
    for (const parser of parsers) {
        const result = ParseParser(moduleProperties, parser, rest, context);
        if (result.length === 0)
            return [];
        buffer.push(result[0]);
        rest = result[1];
    }
    return [buffer, rest];
}
// ------------------------------------------------------------------
// Union
// ------------------------------------------------------------------
// prettier-ignore
function ParseUnion(moduleProperties, parsers, code, context) {
    for (const parser of parsers) {
        const result = ParseParser(moduleProperties, parser, code, context);
        if (result.length === 0)
            continue;
        return result;
    }
    return [];
}
// ------------------------------------------------------------------
// Parser
// ------------------------------------------------------------------
// prettier-ignore
function ParseParser(moduleProperties, parser, code, context) {
    const result = (Guard.IsContext(parser) ? ParseContext(moduleProperties, parser.left, parser.right, code, context) :
        Guard.IsArray(parser) ? ParseArray(moduleProperties, parser.parser, code, context) :
            Guard.IsConst(parser) ? ParseConst(parser.value, code, context) :
                Guard.IsIdent(parser) ? ParseIdent(code, context) :
                    Guard.IsNumber(parser) ? ParseNumber(code, context) :
                        Guard.IsOptional(parser) ? ParseOptional(moduleProperties, parser.parser, code, context) :
                            Guard.IsRef(parser) ? ParseRef(moduleProperties, parser.ref, code, context) :
                                Guard.IsString(parser) ? ParseString(parser.options, code, context) :
                                    Guard.IsTuple(parser) ? ParseTuple(moduleProperties, parser.parsers, code, context) :
                                        Guard.IsUnion(parser) ? ParseUnion(moduleProperties, parser.parsers, code, context) :
                                            []);
    return (result.length === 2
        ? [parser.mapping(result[0], context), result[1]]
        : result);
}
/** Parses content using the given parser */
// prettier-ignore
function Parse(...args) {
    const withModuleProperties = typeof args[1] === 'string' ? false : true;
    const [moduleProperties, parser, content, context] = withModuleProperties
        ? [args[0], args[1], args[2], args[3]]
        : [{}, args[0], args[1], args[2]];
    return ParseParser(moduleProperties, parser, content, context);
}
