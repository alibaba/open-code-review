import * as T from '../type/index.mjs';
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
export function GenericReferenceParameterListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
export function GenericReferenceMapping(input, context) {
    const type = Dereference(context, input[0]);
    const args = input[2];
    return T.Instantiate(type, args);
}
// prettier-ignore
export function GenericArgumentsListMapping(input, context) {
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
export function GenericArgumentsMapping(input, context) {
    return input.length === 3
        ? GenericArgumentsContext(input[1], context)
        : {};
}
// prettier-ignore
export function KeywordStringMapping(input, context) {
    return T.String();
}
// prettier-ignore
export function KeywordNumberMapping(input, context) {
    return T.Number();
}
// prettier-ignore
export function KeywordBooleanMapping(input, context) {
    return T.Boolean();
}
// prettier-ignore
export function KeywordUndefinedMapping(input, context) {
    return T.Undefined();
}
// prettier-ignore
export function KeywordNullMapping(input, context) {
    return T.Null();
}
// prettier-ignore
export function KeywordIntegerMapping(input, context) {
    return T.Integer();
}
// prettier-ignore
export function KeywordBigIntMapping(input, context) {
    return T.BigInt();
}
// prettier-ignore
export function KeywordUnknownMapping(input, context) {
    return T.Unknown();
}
// prettier-ignore
export function KeywordAnyMapping(input, context) {
    return T.Any();
}
// prettier-ignore
export function KeywordNeverMapping(input, context) {
    return T.Never();
}
// prettier-ignore
export function KeywordSymbolMapping(input, context) {
    return T.Symbol();
}
// prettier-ignore
export function KeywordVoidMapping(input, context) {
    return T.Void();
}
// prettier-ignore
export function KeywordMapping(input, context) {
    return input;
}
// prettier-ignore
export function LiteralStringMapping(input, context) {
    return T.Literal(input);
}
// prettier-ignore
export function LiteralNumberMapping(input, context) {
    return T.Literal(parseFloat(input));
}
// prettier-ignore
export function LiteralBooleanMapping(input, context) {
    return T.Literal(input === 'true');
}
// prettier-ignore
export function LiteralMapping(input, context) {
    return input;
}
// prettier-ignore
export function KeyOfMapping(input, context) {
    return input.length > 0;
}
// prettier-ignore
export function IndexArrayMapping(input, context) {
    return input.reduce((result, current) => {
        return current.length === 3
            ? [...result, [current[1]]]
            : [...result, []];
    }, []);
}
// prettier-ignore
export function ExtendsMapping(input, context) {
    return input.length === 6
        ? [input[1], input[3], input[5]]
        : [];
}
// prettier-ignore
export function BaseMapping(input, context) {
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
export function FactorMapping(input, context) {
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
export function ExprTermTailMapping(input, context) {
    return input;
}
// prettier-ignore
export function ExprTermMapping(input, context) {
    const [left, rest] = input;
    return ExprBinaryMapping(left, rest);
}
// prettier-ignore
export function ExprTailMapping(input, context) {
    return input;
}
// prettier-ignore
export function ExprMapping(input, context) {
    const [left, rest] = input;
    return ExprBinaryMapping(left, rest);
}
// prettier-ignore
export function TypeMapping(input, context) {
    return input;
}
// prettier-ignore
export function PropertyKeyMapping(input, context) {
    return input;
}
// prettier-ignore
export function ReadonlyMapping(input, context) {
    return input.length > 0;
}
// prettier-ignore
export function OptionalMapping(input, context) {
    return input.length > 0;
}
// prettier-ignore
export function PropertyMapping(input, context) {
    const [isReadonly, key, isOptional, _colon, type] = input;
    return {
        [key]: (isReadonly && isOptional ? T.ReadonlyOptional(type) :
            isReadonly && !isOptional ? T.Readonly(type) :
                !isReadonly && isOptional ? T.Optional(type) :
                    type)
    };
}
// prettier-ignore
export function PropertyDelimiterMapping(input, context) {
    return input;
}
// prettier-ignore
export function PropertyListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
export function ObjectMapping(input, context) {
    const propertyList = input[1];
    return T.Object(propertyList.reduce((result, property) => {
        return { ...result, ...property };
    }, {}));
}
// prettier-ignore
export function ElementListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
export function TupleMapping(input, context) {
    return T.Tuple(input[1]);
}
// prettier-ignore
export function ParameterMapping(input, context) {
    const [_ident, _colon, type] = input;
    return type;
}
// prettier-ignore
export function ParameterListMapping(input, context) {
    return Delimited(input);
}
// prettier-ignore
export function FunctionMapping(input, context) {
    const [_lparan, parameterList, _rparan, _arrow, returnType] = input;
    return T.Function(parameterList, returnType);
}
// prettier-ignore
export function ConstructorMapping(input, context) {
    const [_new, _lparan, parameterList, _rparan, _arrow, instanceType] = input;
    return T.Constructor(parameterList, instanceType);
}
// prettier-ignore
export function MappedMapping(input, context) {
    const [_lbrace, _lbracket, _key, _in, _right, _rbracket, _colon, _type] = input;
    return T.Literal('Mapped types not supported');
}
// prettier-ignore
export function AsyncIteratorMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.AsyncIterator(type);
}
// prettier-ignore
export function IteratorMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Iterator(type);
}
// prettier-ignore
export function ArgumentMapping(input, context) {
    return T.KindGuard.IsLiteralNumber(input[2])
        ? T.Argument(Math.trunc(input[2].const))
        : T.Never();
}
// prettier-ignore
export function AwaitedMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Awaited(type);
}
// prettier-ignore
export function ArrayMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Array(type);
}
// prettier-ignore
export function RecordMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Record(key, type);
}
// prettier-ignore
export function PromiseMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Promise(type);
}
// prettier-ignore
export function ConstructorParametersMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.ConstructorParameters(type);
}
// prettier-ignore
export function FunctionParametersMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Parameters(type);
}
// prettier-ignore
export function InstanceTypeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.InstanceType(type);
}
// prettier-ignore
export function ReturnTypeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.ReturnType(type);
}
// prettier-ignore
export function PartialMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Partial(type);
}
// prettier-ignore
export function RequiredMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Required(type);
}
// prettier-ignore
export function PickMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Pick(key, type);
}
// prettier-ignore
export function OmitMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Omit(key, type);
}
// prettier-ignore
export function ExcludeMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Exclude(key, type);
}
// prettier-ignore
export function ExtractMapping(input, context) {
    const [_name, _langle, key, _comma, type, _rangle] = input;
    return T.Extract(key, type);
}
// prettier-ignore
export function UppercaseMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Uppercase(type);
}
// prettier-ignore
export function LowercaseMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Lowercase(type);
}
// prettier-ignore
export function CapitalizeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Capitalize(type);
}
// prettier-ignore
export function UncapitalizeMapping(input, context) {
    const [_name, _langle, type, _rangle] = input;
    return T.Uncapitalize(type);
}
// prettier-ignore
export function DateMapping(input, context) {
    return T.Date();
}
// prettier-ignore
export function Uint8ArrayMapping(input, context) {
    return T.Uint8Array();
}
// prettier-ignore
export function ReferenceMapping(input, context) {
    const target = Dereference(context, input);
    return target;
}
