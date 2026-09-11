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
exports.GenericReferenceParameterListMapping = GenericReferenceParameterListMapping;
exports.GenericReferenceMapping = GenericReferenceMapping;
exports.GenericArgumentsListMapping = GenericArgumentsListMapping;
exports.GenericArgumentsMapping = GenericArgumentsMapping;
exports.KeywordStringMapping = KeywordStringMapping;
exports.KeywordNumberMapping = KeywordNumberMapping;
exports.KeywordBooleanMapping = KeywordBooleanMapping;
exports.KeywordUndefinedMapping = KeywordUndefinedMapping;
exports.KeywordNullMapping = KeywordNullMapping;
exports.KeywordIntegerMapping = KeywordIntegerMapping;
exports.KeywordBigIntMapping = KeywordBigIntMapping;
exports.KeywordUnknownMapping = KeywordUnknownMapping;
exports.KeywordAnyMapping = KeywordAnyMapping;
exports.KeywordNeverMapping = KeywordNeverMapping;
exports.KeywordSymbolMapping = KeywordSymbolMapping;
exports.KeywordVoidMapping = KeywordVoidMapping;
exports.KeywordMapping = KeywordMapping;
exports.LiteralStringMapping = LiteralStringMapping;
exports.LiteralNumberMapping = LiteralNumberMapping;
exports.LiteralBooleanMapping = LiteralBooleanMapping;
exports.LiteralMapping = LiteralMapping;
exports.KeyOfMapping = KeyOfMapping;
exports.IndexArrayMapping = IndexArrayMapping;
exports.ExtendsMapping = ExtendsMapping;
exports.BaseMapping = BaseMapping;
exports.FactorMapping = FactorMapping;
exports.ExprTermTailMapping = ExprTermTailMapping;
exports.ExprTermMapping = ExprTermMapping;
exports.ExprTailMapping = ExprTailMapping;
exports.ExprMapping = ExprMapping;
exports.TypeMapping = TypeMapping;
exports.PropertyKeyMapping = PropertyKeyMapping;
exports.ReadonlyMapping = ReadonlyMapping;
exports.OptionalMapping = OptionalMapping;
exports.PropertyMapping = PropertyMapping;
exports.PropertyDelimiterMapping = PropertyDelimiterMapping;
exports.PropertyListMapping = PropertyListMapping;
exports.ObjectMapping = ObjectMapping;
exports.ElementListMapping = ElementListMapping;
exports.TupleMapping = TupleMapping;
exports.ParameterMapping = ParameterMapping;
exports.ParameterListMapping = ParameterListMapping;
exports.FunctionMapping = FunctionMapping;
exports.ConstructorMapping = ConstructorMapping;
exports.MappedMapping = MappedMapping;
exports.AsyncIteratorMapping = AsyncIteratorMapping;
exports.IteratorMapping = IteratorMapping;
exports.ArgumentMapping = ArgumentMapping;
exports.AwaitedMapping = AwaitedMapping;
exports.ArrayMapping = ArrayMapping;
exports.RecordMapping = RecordMapping;
exports.PromiseMapping = PromiseMapping;
exports.ConstructorParametersMapping = ConstructorParametersMapping;
exports.FunctionParametersMapping = FunctionParametersMapping;
exports.InstanceTypeMapping = InstanceTypeMapping;
exports.ReturnTypeMapping = ReturnTypeMapping;
exports.PartialMapping = PartialMapping;
exports.RequiredMapping = RequiredMapping;
exports.PickMapping = PickMapping;
exports.OmitMapping = OmitMapping;
exports.ExcludeMapping = ExcludeMapping;
exports.ExtractMapping = ExtractMapping;
exports.UppercaseMapping = UppercaseMapping;
exports.LowercaseMapping = LowercaseMapping;
exports.CapitalizeMapping = CapitalizeMapping;
exports.UncapitalizeMapping = UncapitalizeMapping;
exports.DateMapping = DateMapping;
exports.Uint8ArrayMapping = Uint8ArrayMapping;
exports.ReferenceMapping = ReferenceMapping;
const T = __importStar(require("../type/index"));
// prettier-ignore
const Dereference = (context, key) => {
    return key in context ? context[key] : T.Ref(key);
};
// prettier-ignore
const DelimitedDecode = (input, result = []) => {
    return input.reduce((result, left) => {
        return T.ValueGuard.IsArray(left) && left.length === 2
            ? [...result, left[0]]
            : [...result, left];
    }, []);
};
// prettier-ignore
const Delimited = (input) => {
    const [left, right] = input;
    return DelimitedDecode([...left, ...right]);
};
// prettier-ignore
function GenericReferenceParameterListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
function GenericReferenceMapping(input, context) {
    const type = Dereference(context, input[0]);
    const args = input[2];
    return T.Instantiate(type, args);
}
// prettier-ignore
function GenericArgumentsListMapping(input, context) {
    return Delimited(input);
}
// ...
// prettier-ignore
const GenericArgumentsContext = (_arguments, context) => {
    return _arguments.reduce((result, arg, index) => {
        return { ...result, [arg]: T.Argument(index) };
    }, context);
};
// prettier-ignore
function GenericArgumentsMapping(input, context) {
    return input.length === 3
        ? GenericArgumentsContext(input[1], context)
        : {};
}
// prettier-ignore
function KeywordStringMapping(input, context) {
    return T.String();
}
// prettier-ignore
function KeywordNumberMapping(input, context) {
    return T.Number();
}
// prettier-ignore
function KeywordBooleanMapping(input, context) {
    return T.Boolean();
}
// prettier-ignore
function KeywordUndefinedMapping(input, context) {
    return T.Undefined();
}
// prettier-ignore
function KeywordNullMapping(input, context) {
    return T.Null();
}
// prettier-ignore
function KeywordIntegerMapping(input, context) {
    return T.Integer();
}
// prettier-ignore
function KeywordBigIntMapping(input, context) {
    return T.BigInt();
}
// prettier-ignore
function KeywordUnknownMapping(input, context) {
    return T.Unknown();
}
// prettier-ignore
function KeywordAnyMapping(input, context) {
    return T.Any();
}
// prettier-ignore
function KeywordNeverMapping(input, context) {
    return T.Never();
}
// prettier-ignore
function KeywordSymbolMapping(input, context) {
    return T.Symbol();
}
// prettier-ignore
function KeywordVoidMapping(input, context) {
    return T.Void();
}
// prettier-ignore
function KeywordMapping(input, context) {
    return input;
}
// prettier-ignore
function LiteralStringMapping(input, context) {
    return T.Literal(input);
}
// prettier-ignore
function LiteralNumberMapping(input, context) {
    return T.Literal(parseFloat(input));
}
// prettier-ignore
function LiteralBooleanMapping(input, context) {
    return T.Literal(input === 'true');
}
// prettier-ignore
function LiteralMapping(input, context) {
    return input;
}
// prettier-ignore
function KeyOfMapping(input, context) {
    return input.length > 0;
}
// prettier-ignore
function IndexArrayMapping(input, context) {
    return input.reduce((result, current) => {
        return current.length === 3
            ? [...result, [current[1]]]
            : [...result, []];
    }, []);
}
// prettier-ignore
function ExtendsMapping(input, context) {
    return input.length === 6
        ? [input[1], input[3], input[5]]
        : [];
}
// prettier-ignore
function BaseMapping(input, context) {
    return T.ValueGuard.IsArray(input) && input.length === 3 ? input[1] : input;
}
// ...
// prettier-ignore
const FactorIndexArray = (Type, indexArray) => {
    return indexArray.reduceRight((result, right) => {
        const _right = right;
        return (_right.length === 1 ? T.Index(result, _right[0]) :
            _right.length === 0 ? T.Array(result, _right[0]) :
                T.Never());
    }, Type);
};
// prettier-ignore
const FactorExtends = (Type, Extends) => {
    return Extends.length === 3
        ? T.Extends(Type, Extends[0], Extends[1], Extends[2])
        : Type;
};
// prettier-ignore
function FactorMapping(input, context) {
    const [KeyOf, Type, IndexArray, Extends] = input;
    return KeyOf
        ? FactorExtends(T.KeyOf(FactorIndexArray(Type, IndexArray)), Extends)
        : FactorExtends(FactorIndexArray(Type, IndexArray), Extends);
}
// prettier-ignore
function ExprBinaryMapping(Left, Rest) {
    return (Rest.length === 3 ? (() => {
        const [Operator, Right, Next] = Rest;
        const Schema = ExprBinaryMapping(Right, Next);
        if (Operator === '&') {
            return T.TypeGuard.IsIntersect(Schema)
                ? T.Intersect([Left, ...Schema.allOf])
                : T.Intersect([Left, Schema]);
        }
        if (Operator === '|') {
            return T.TypeGuard.IsUnion(Schema)
                ? T.Union([Left, ...Schema.anyOf])
                : T.Union([Left, Schema]);
        }
        throw 1;
    })() : Left);
}
// prettier-ignore
function ExprTermTailMapping(input, context) {
    return input;
}
// prettier-ignore
function ExprTermMapping(input, context) {
    const [left, rest] = input;
    return ExprBinaryMapping(left, rest);
}
// prettier-ignore
function ExprTailMapping(input, context) {
    return input;
}
// prettier-ignore
function ExprMapping(input, context) {
    const [left, rest] = input;
    return ExprBinaryMapping(left, rest);
}
// prettier-ignore
function TypeMapping(input, context) {
    return input;
}
// prettier-ignore
function PropertyKeyMapping(input, context) {
    return input;
}
// prettier-ignore
function ReadonlyMapping(input, context) {
    return input.length > 0;
}
// prettier-ignore
function OptionalMapping(input, context) {
    return input.length > 0;
}
// prettier-ignore
function PropertyMapping(input, context) {
    const [isReadonly, key, isOptional, _colon, type] = input;
    return {
        [key]: (isReadonly && isOptional ? T.ReadonlyOptional(type) :
            isReadonly && !isOptional ? T.Readonly(type) :
                !isReadonly && isOptional ? T.Optional(type) :
                    type)
    };
}
// prettier-ignore
function PropertyDelimiterMapping(input, context) {
    return input;
}
// prettier-ignore
function PropertyListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
function ObjectMapping(input, context) {
    const propertyList = input[1];
    return T.Object(propertyList.reduce((result, property) => {
        return { ...result, ...property };
    }, {}));
}
// prettier-ignore
function ElementListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
function TupleMapping(input, context) {
    return T.Tuple(input[1]);
}
// prettier-ignore
function ParameterMapping(input, context) {
    const [_ident, _colon, type] = input;
    return type;
}
// prettier-ignore
function ParameterListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
function FunctionMapping(input, context) {
    const [_lparan, parameterList, _rparan, _arrow, returnType] = input;
    return T.Function(parameterList, returnType);
}
// prettier-ignore
function ConstructorMapping(input, context) {
    const [_new, _lparan, parameterList, _rparan, _arrow, instanceType] = input;
    return T.Constructor(parameterList, instanceType);
}
// prettier-ignore
function MappedMapping(input, context) {
    const [_lbrace, _lbracket, _key, _in, _right, _rbracket, _colon, _type] = input;
    return T.Literal('Mapped types not supported');
}
// prettier-ignore
function AsyncIteratorMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.AsyncIterator(type);
}
// prettier-ignore
function IteratorMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Iterator(type);
}
// prettier-ignore
function ArgumentMapping(input, context) {
    return T.KindGuard.IsLiteralNumber(input[2])
        ? T.Argument(Math.trunc(input[2].const))
        : T.Never();
}
// prettier-ignore
function AwaitedMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Awaited(type);
}
// prettier-ignore
function ArrayMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Array(type);
}
// prettier-ignore
function RecordMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Record(key, type);
}
// prettier-ignore
function PromiseMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Promise(type);
}
// prettier-ignore
function ConstructorParametersMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.ConstructorParameters(type);
}
// prettier-ignore
function FunctionParametersMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Parameters(type);
}
// prettier-ignore
function InstanceTypeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.InstanceType(type);
}
// prettier-ignore
function ReturnTypeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.ReturnType(type);
}
// prettier-ignore
function PartialMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Partial(type);
}
// prettier-ignore
function RequiredMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Required(type);
}
// prettier-ignore
function PickMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Pick(key, type);
}
// prettier-ignore
function OmitMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Omit(key, type);
}
// prettier-ignore
function ExcludeMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Exclude(key, type);
}
// prettier-ignore
function ExtractMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Extract(key, type);
}
// prettier-ignore
function UppercaseMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Uppercase(type);
}
// prettier-ignore
function LowercaseMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Lowercase(type);
}
// prettier-ignore
function CapitalizeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Capitalize(type);
}
// prettier-ignore
function UncapitalizeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Uncapitalize(type);
}
// prettier-ignore
function DateMapping(input, context) {
    return T.Date();
}
// prettier-ignore
function Uint8ArrayMapping(input, context) {
    return T.Uint8Array();
}
// prettier-ignore
function ReferenceMapping(input, context) {
    const target = Dereference(context, input);
    return target;
}
