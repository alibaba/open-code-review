import { TransformEncode, TransformDecode, HasTransform, TransformDecodeCheckError, TransformEncodeCheckError } from '../value/transform/index.mjs';
import { Errors } from '../errors/index.mjs';
import { TypeSystemPolicy, Evaluate } from '../system/index.mjs';
import { TypeBoxError } from '../type/error/index.mjs';
import { Deref } from '../value/deref/index.mjs';
import { Hash } from '../value/hash/index.mjs';
import { Kind } from '../type/symbols/index.mjs';
import { TypeRegistry, FormatRegistry } from '../type/registry/index.mjs';
import { KeyOfPattern } from '../type/keyof/index.mjs';
import { ExtendsUndefinedCheck } from '../type/extends/extends-undefined.mjs';
import { Never } from '../type/never/index.mjs';
import { Ref } from '../type/ref/index.mjs';
// ------------------------------------------------------------------
// ValueGuard
// ------------------------------------------------------------------
import { IsArray, IsString, IsNumber, IsBigInt } from '../value/guard/index.mjs';
// ------------------------------------------------------------------
// TypeGuard
// ------------------------------------------------------------------
import { IsSchema } from '../type/guard/type.mjs';
// ------------------------------------------------------------------
// TypeCheck
// ------------------------------------------------------------------
export class TypeCheck {
    constructor(schema, references, checkFunc, code) {
        this.schema = schema;
        this.references = references;
        this.checkFunc = checkFunc;
        this.code = code;
        this.hasTransform = HasTransform(schema, references);
    }
    /** Returns the generated assertion code used to validate this type. */
    Code() {
        return this.code;
    }
    /** Returns the schema type used to validate */
    Schema() {
        return this.schema;
    }
    /** Returns reference types used to validate */
    References() {
        return this.references;
    }
    /** Returns an iterator for each error in this value. */
    Errors(value) {
        return Errors(this.schema, this.references, value);
    }
    /** Returns true if the value matches the compiled type. */
    Check(value) {
        return this.checkFunc(value);
    }
    /** Decodes a value or throws if error */
    Decode(value) {
        if (!this.checkFunc(value))
            throw new TransformDecodeCheckError(this.schema, value, this.Errors(value).First());
        return (this.hasTransform ? TransformDecode(this.schema, this.references, value) : value);
    }
    /** Encodes a value or throws if error */
    Encode(value) {
        const encoded = this.hasTransform ? TransformEncode(this.schema, this.references, value) : value;
        if (!this.checkFunc(encoded))
            throw new TransformEncodeCheckError(this.schema, value, this.Errors(value).First());
        return encoded;
    }
}
// ------------------------------------------------------------------
// Character
// ------------------------------------------------------------------
var Character;
(function (Character) {
    function DollarSign(code) {
        return code === 36;
    }
    Character.DollarSign = DollarSign;
    function IsUnderscore(code) {
        return code === 95;
    }
    Character.IsUnderscore = IsUnderscore;
    function IsAlpha(code) {
        return (code >= 65 && code <= 90) || (code >= 97 && code <= 122);
    }
    Character.IsAlpha = IsAlpha;
    function IsNumeric(code) {
        return code >= 48 && code <= 57;
    }
    Character.IsNumeric = IsNumeric;
})(Character || (Character = {}));
// ------------------------------------------------------------------
// Identifier
// ------------------------------------------------------------------
var Identifier;
(function (Identifier) {
    function Encode($id) {
        const buffer = [];
        for (let i = 0; i < $id.length; i++) {
            const code = $id.charCodeAt(i);
            if (Character.IsNumeric(code) || Character.IsAlpha(code)) {
                buffer.push($id.charAt(i));
            }
            else {
                buffer.push(`_${code}_`);
            }
        }
        return buffer.join('').replace(/__/g, '_');
    }
    Identifier.Encode = Encode;
})(Identifier || (Identifier = {}));
// ------------------------------------------------------------------
// StringConstant
// ------------------------------------------------------------------
function StringConstant(value) {
    if (!IsString(value))
        throw Error('ConstantString: Not a String');
    const canonical = JSON.stringify(value).slice(1, -1);
    const escaped = canonical.replace(/'/g, "\\'");
    return `'${escaped}'`;
}
// ------------------------------------------------------------------
// MemberExpression
// ------------------------------------------------------------------
function MemberExpression(value, key) {
    return `${value}[${StringConstant(key)}]`;
}
// ------------------------------------------------------------------
// Errors
// ------------------------------------------------------------------
export class TypeCompilerUnknownTypeError extends TypeBoxError {
    constructor(schema) {
        super('Unknown type');
        this.schema = schema;
    }
}
export class TypeCompilerTypeGuardError extends TypeBoxError {
    constructor(schema) {
        super('Preflight validation check failed to guard for the given schema');
        this.schema = schema;
    }
}
// ------------------------------------------------------------------
// Policy
// ------------------------------------------------------------------
export var Policy;
(function (Policy) {
    function IsExactOptionalProperty(value, key, expression) {
        return TypeSystemPolicy.ExactOptionalPropertyTypes ? `(${StringConstant(key)} in ${value} ? ${expression} : true)` : `(${MemberExpression(value, key)} !== undefined ? ${expression} : true)`;
    }
    Policy.IsExactOptionalProperty = IsExactOptionalProperty;
    function IsObjectLike(value) {
        return !TypeSystemPolicy.AllowArrayObject ? `(typeof ${value} === 'object' && ${value} !== null && !Array.isArray(${value}))` : `(typeof ${value} === 'object' && ${value} !== null)`;
    }
    Policy.IsObjectLike = IsObjectLike;
    function IsRecordLike(value) {
        return !TypeSystemPolicy.AllowArrayObject
            ? `(typeof ${value} === 'object' && ${value} !== null && !Array.isArray(${value}) && !(${value} instanceof Date) && !(${value} instanceof Uint8Array))`
            : `(typeof ${value} === 'object' && ${value} !== null && !(${value} instanceof Date) && !(${value} instanceof Uint8Array))`;
    }
    Policy.IsRecordLike = IsRecordLike;
    function IsNumberLike(value) {
        return TypeSystemPolicy.AllowNaN ? `typeof ${value} === 'number'` : `Number.isFinite(${value})`;
    }
    Policy.IsNumberLike = IsNumberLike;
    function IsVoidLike(value) {
        return TypeSystemPolicy.AllowNullVoid ? `(${value} === undefined || ${value} === null)` : `${value} === undefined`;
    }
    Policy.IsVoidLike = IsVoidLike;
})(Policy || (Policy = {}));
/** Compiles Types for Runtime Type Checking */
export var TypeCompiler;
(function (TypeCompiler) {
    // ----------------------------------------------------------------
    // Guards
    // ----------------------------------------------------------------
    function IsAnyOrUnknown(schema) {
        return schema[Kind] === 'Any' || schema[Kind] === 'Unknown';
    }
    // ----------------------------------------------------------------
    // Types
    // ----------------------------------------------------------------
    function* FromAny(schema, references, value) {
        yield 'true';
    }
    function* FromArgument(schema, references, value) {
        yield 'true';
    }
    function* FromArray(schema, references, value) {
        yield `Array.isArray(${value})`;
        const [parameter, accumulator] = [CreateParameter('value', 'any'), CreateParameter('acc', 'number')];
        if (IsNumber(schema.maxItems))
            yield `${value}.length <= ${schema.maxItems}`;
        if (IsNumber(schema.minItems))
            yield `${value}.length >= ${schema.minItems}`;
        const elementExpression = CreateExpression(schema.items, references, 'value');
        // yield `${value}.every((${parameter}) => ${elementExpression})` // issue: 1519
        yield `((array) => { for(const ${parameter} of array) if(!(${elementExpression})) { return false }; return true; })(${value})`;
        if (IsSchema(schema.contains) || IsNumber(schema.minContains) || IsNumber(schema.maxContains)) {
            const containsSchema = IsSchema(schema.contains) ? schema.contains : Never();
            const checkExpression = CreateExpression(containsSchema, references, 'value');
            const checkMinContains = IsNumber(schema.minContains) ? [`(count >= ${schema.minContains})`] : [];
            const checkMaxContains = IsNumber(schema.maxContains) ? [`(count <= ${schema.maxContains})`] : [];
            const checkCount = `const count = value.reduce((${accumulator}, ${parameter}) => ${checkExpression} ? acc + 1 : acc, 0)`;
            const check = [`(count > 0)`, ...checkMinContains, ...checkMaxContains].join(' && ');
            yield `((${parameter}) => { ${checkCount}; return ${check}})(${value})`;
        }
        if (schema.uniqueItems === true) {
            const check = `const hashed = hash(element); if(set.has(hashed)) { return false } else { set.add(hashed) } } return true`;
            const block = `const set = new Set(); for(const element of value) { ${check} }`;
            yield `((${parameter}) => { ${block} )(${value})`;
        }
    }
    function* FromAsyncIterator(schema, references, value) {
        yield `(typeof value === 'object' && Symbol.asyncIterator in ${value})`;
    }
    function* FromBigInt(schema, references, value) {
        yield `(typeof ${value} === 'bigint')`;
        if (IsBigInt(schema.exclusiveMaximum))
            yield `${value} < BigInt(${schema.exclusiveMaximum})`;
        if (IsBigInt(schema.exclusiveMinimum))
            yield `${value} > BigInt(${schema.exclusiveMinimum})`;
        if (IsBigInt(schema.maximum))
            yield `${value} <= BigInt(${schema.maximum})`;
        if (IsBigInt(schema.minimum))
            yield `${value} >= BigInt(${schema.minimum})`;
        if (IsBigInt(schema.multipleOf))
            yield `(${value} % BigInt(${schema.multipleOf})) === 0`;
    }
    function* FromBoolean(schema, references, value) {
        yield `(typeof ${value} === 'boolean')`;
    }
    function* FromConstructor(schema, references, value) {
        yield* Visit(schema.returns, references, `${value}.prototype`);
    }
    function* FromDate(schema, references, value) {
        yield `(${value} instanceof Date) && Number.isFinite(${value}.getTime())`;
        if (IsNumber(schema.exclusiveMaximumTimestamp))
            yield `${value}.getTime() < ${schema.exclusiveMaximumTimestamp}`;
        if (IsNumber(schema.exclusiveMinimumTimestamp))
            yield `${value}.getTime() > ${schema.exclusiveMinimumTimestamp}`;
        if (IsNumber(schema.maximumTimestamp))
            yield `${value}.getTime() <= ${schema.maximumTimestamp}`;
        if (IsNumber(schema.minimumTimestamp))
            yield `${value}.getTime() >= ${schema.minimumTimestamp}`;
        if (IsNumber(schema.multipleOfTimestamp))
            yield `(${value}.getTime() % ${schema.multipleOfTimestamp}) === 0`;
    }
    function* FromFunction(schema, references, value) {
        yield `(typeof ${value} === 'function')`;
    }
    function* FromImport(schema, references, value) {
        const members = globalThis.Object.getOwnPropertyNames(schema.$defs).reduce((result, key) => {
            return [...result, schema.$defs[key]];
        }, []);
        yield* Visit(Ref(schema.$ref), [...references, ...members], value);
    }
    function* FromInteger(schema, references, value) {
        yield `Number.isInteger(${value})`;
        if (IsNumber(schema.exclusiveMaximum))
            yield `${value} < ${schema.exclusiveMaximum}`;
        if (IsNumber(schema.exclusiveMinimum))
            yield `${value} > ${schema.exclusiveMinimum}`;
        if (IsNumber(schema.maximum))
            yield `${value} <= ${schema.maximum}`;
        if (IsNumber(schema.minimum))
            yield `${value} >= ${schema.minimum}`;
        if (IsNumber(schema.multipleOf))
            yield `(${value} % ${schema.multipleOf}) === 0`;
    }
    function* FromIntersect(schema, references, value) {
        const check1 = schema.allOf.map((schema) => CreateExpression(schema, references, value)).join(' && ');
        if (schema.unevaluatedProperties === false) {
            const keyCheck = CreateVariable(`${new RegExp(KeyOfPattern(schema))};`);
            const check2 = `Object.getOwnPropertyNames(${value}).every(key => ${keyCheck}.test(key))`;
            yield `(${check1} && ${check2})`;
        }
        else if (IsSchema(schema.unevaluatedProperties)) {
            const keyCheck = CreateVariable(`${new RegExp(KeyOfPattern(schema))};`);
            const check2 = `Object.getOwnPropertyNames(${value}).every(key => ${keyCheck}.test(key) || ${CreateExpression(schema.unevaluatedProperties, references, `${value}[key]`)})`;
            yield `(${check1} && ${check2})`;
        }
        else {
            yield `(${check1})`;
        }
    }
    function* FromIterator(schema, references, value) {
        yield `(typeof value === 'object' && Symbol.iterator in ${value})`;
    }
    function* FromLiteral(schema, references, value) {
        if (typeof schema.const === 'number' || typeof schema.const === 'boolean') {
            yield `(${value} === ${schema.const})`;
        }
        else if (typeof schema.const === 'string') {
            yield `(${value} === ${StringConstant(schema.const)})`;
        }
        else {
            throw Error('Invalid Literal Value');
        }
    }
    function* FromNever(schema, references, value) {
        yield `false`;
    }
    function* FromNot(schema, references, value) {
        const expression = CreateExpression(schema.not, references, value);
        yield `(!${expression})`;
    }
    function* FromNull(schema, references, value) {
        yield `(${value} === null)`;
    }
    function* FromNumber(schema, references, value) {
        yield Policy.IsNumberLike(value);
        if (IsNumber(schema.exclusiveMaximum))
            yield `${value} < ${schema.exclusiveMaximum}`;
        if (IsNumber(schema.exclusiveMinimum))
            yield `${value} > ${schema.exclusiveMinimum}`;
        if (IsNumber(schema.maximum))
            yield `${value} <= ${schema.maximum}`;
        if (IsNumber(schema.minimum))
            yield `${value} >= ${schema.minimum}`;
        if (IsNumber(schema.multipleOf))
            yield `(${value} % ${schema.multipleOf}) === 0`;
    }
    function* FromObject(schema, references, value) {
        yield Policy.IsObjectLike(value);
        if (IsNumber(schema.minProperties))
            yield `Object.getOwnPropertyNames(${value}).length >= ${schema.minProperties}`;
        if (IsNumber(schema.maxProperties))
            yield `Object.getOwnPropertyNames(${value}).length <= ${schema.maxProperties}`;
        const knownKeys = Object.getOwnPropertyNames(schema.properties);
        for (const knownKey of knownKeys) {
            const memberExpression = MemberExpression(value, knownKey);
            const property = schema.properties[knownKey];
            if (schema.required && schema.required.includes(knownKey)) {
                yield* Visit(property, references, memberExpression);
                if (ExtendsUndefinedCheck(property) || IsAnyOrUnknown(property))
                    yield `(${StringConstant(knownKey)} in ${value})`;
            }
            else {
                const expression = CreateExpression(property, references, memberExpression);
                yield Policy.IsExactOptionalProperty(value, knownKey, expression);
            }
        }
        if (schema.additionalProperties === false) {
            if (schema.required && schema.required.length === knownKeys.length) {
                yield `Object.getOwnPropertyNames(${value}).length === ${knownKeys.length}`;
            }
            else {
                const keys = `[${knownKeys.map((key) => `${StringConstant(key)}`).join(', ')}]`;
                yield `Object.getOwnPropertyNames(${value}).every(key => ${keys}.includes(key))`;
            }
        }
        if (typeof schema.additionalProperties === 'object') {
            const expression = CreateExpression(schema.additionalProperties, references, `${value}[key]`);
            const keys = `[${knownKeys.map((key) => `${StringConstant(key)}`).join(', ')}]`;
            yield `(Object.getOwnPropertyNames(${value}).every(key => ${keys}.includes(key) || ${expression}))`;
        }
    }
    function* FromPromise(schema, references, value) {
        yield `${value} instanceof Promise`;
    }
    function* FromRecord(schema, references, value) {
        yield Policy.IsRecordLike(value);
        if (IsNumber(schema.minProperties))
            yield `Object.getOwnPropertyNames(${value}).length >= ${schema.minProperties}`;
        if (IsNumber(schema.maxProperties))
            yield `Object.getOwnPropertyNames(${value}).length <= ${schema.maxProperties}`;
        const [patternKey, patternSchema] = Object.entries(schema.patternProperties)[0];
        const variable = CreateVariable(`${new RegExp(patternKey)}`);
        const check1 = CreateExpression(patternSchema, references, 'value');
        const check2 = IsSchema(schema.additionalProperties) ? CreateExpression(schema.additionalProperties, references, value) : schema.additionalProperties === false ? 'false' : 'true';
        const expression = `(${variable}.test(key) ? ${check1} : ${check2})`;
        yield `(Object.entries(${value}).every(([key, value]) => ${expression}))`;
    }
    function* FromRef(schema, references, value) {
        const target = Deref(schema, references);
        // Reference: If we have seen this reference before we can just yield and return the function call.
        // If this isn't the case we defer to visit to generate and set the function for subsequent passes.
        if (state.functions.has(schema.$ref))
            return yield `${CreateFunctionName(schema.$ref)}(${value})`;
        yield* Visit(target, references, value);
    }
    function* FromRegExp(schema, references, value) {
        const variable = CreateVariable(`${new RegExp(schema.source, schema.flags)};`);
        yield `(typeof ${value} === 'string')`;
        if (IsNumber(schema.maxLength))
            yield `${value}.length <= ${schema.maxLength}`;
        if (IsNumber(schema.minLength))
            yield `${value}.length >= ${schema.minLength}`;
        yield `${variable}.test(${value})`;
    }
    function* FromString(schema, references, value) {
        yield `(typeof ${value} === 'string')`;
        if (IsNumber(schema.maxLength))
            yield `${value}.length <= ${schema.maxLength}`;
        if (IsNumber(schema.minLength))
            yield `${value}.length >= ${schema.minLength}`;
        if (schema.pattern !== undefined) {
            const variable = CreateVariable(`${new RegExp(schema.pattern)};`);
            yield `${variable}.test(${value})`;
        }
        if (schema.format !== undefined) {
            yield `format(${StringConstant(schema.format)}, ${value})`;
        }
    }
    function* FromSymbol(schema, references, value) {
        yield `(typeof ${value} === 'symbol')`;
    }
    function* FromTemplateLiteral(schema, references, value) {
        yield `(typeof ${value} === 'string')`;
        const variable = CreateVariable(`${new RegExp(schema.pattern)};`);
        yield `${variable}.test(${value})`;
    }
    function* FromThis(schema, references, value) {
        // Note: This types are assured to be hoisted prior to this call. Just yield the function.
        yield `${CreateFunctionName(schema.$ref)}(${value})`;
    }
    function* FromTuple(schema, references, value) {
        yield `Array.isArray(${value})`;
        if (schema.items === undefined)
            return yield `${value}.length === 0`;
        yield `(${value}.length === ${schema.maxItems})`;
        for (let i = 0; i < schema.items.length; i++) {
            const expression = CreateExpression(schema.items[i], references, `${value}[${i}]`);
            yield `${expression}`;
        }
    }
    function* FromUndefined(schema, references, value) {
        yield `${value} === undefined`;
    }
    function* FromUnion(schema, references, value) {
        const expressions = schema.anyOf.map((schema) => CreateExpression(schema, references, value));
        yield `(${expressions.join(' || ')})`;
    }
    function* FromUint8Array(schema, references, value) {
        yield `${value} instanceof Uint8Array`;
        if (IsNumber(schema.maxByteLength))
            yield `(${value}.length <= ${schema.maxByteLength})`;
        if (IsNumber(schema.minByteLength))
            yield `(${value}.length >= ${schema.minByteLength})`;
    }
    function* FromUnknown(schema, references, value) {
        yield 'true';
    }
    function* FromVoid(schema, references, value) {
        yield Policy.IsVoidLike(value);
    }
    function* FromKind(schema, references, value) {
        const instance = state.instances.size;
        state.instances.set(instance, schema);
        yield `kind(${StringConstant(schema[Kind])}, ${instance}, ${value})`;
    }
    function* Visit(schema, references, value, useHoisting = true) {
        const references_ = IsString(schema.$id) ? [...references, schema] : references;
        const schema_ = schema;
        // --------------------------------------------------------------
        // Hoisting
        // --------------------------------------------------------------
        if (useHoisting && IsString(schema.$id)) {
            const functionName = CreateFunctionName(schema.$id);
            if (state.functions.has(functionName)) {
                return yield `${functionName}(${value})`;
            }
            else {
                // Note: In the case of cyclic types, we need to create a 'functions' record
                // to prevent infinitely re-visiting the CreateFunction. Subsequent attempts
                // to visit will be caught by the above condition.
                state.functions.set(functionName, '<deferred>');
                const functionCode = CreateFunction(functionName, schema, references, 'value', false);
                state.functions.set(functionName, functionCode);
                return yield `${functionName}(${value})`;
            }
        }
        switch (schema_[Kind]) {
            case 'Any':
                return yield* FromAny(schema_, references_, value);
            case 'Argument':
                return yield* FromArgument(schema_, references_, value);
            case 'Array':
                return yield* FromArray(schema_, references_, value);
            case 'AsyncIterator':
                return yield* FromAsyncIterator(schema_, references_, value);
            case 'BigInt':
                return yield* FromBigInt(schema_, references_, value);
            case 'Boolean':
                return yield* FromBoolean(schema_, references_, value);
            case 'Constructor':
                return yield* FromConstructor(schema_, references_, value);
            case 'Date':
                return yield* FromDate(schema_, references_, value);
            case 'Function':
                return yield* FromFunction(schema_, references_, value);
            case 'Import':
                return yield* FromImport(schema_, references_, value);
            case 'Integer':
                return yield* FromInteger(schema_, references_, value);
            case 'Intersect':
                return yield* FromIntersect(schema_, references_, value);
            case 'Iterator':
                return yield* FromIterator(schema_, references_, value);
            case 'Literal':
                return yield* FromLiteral(schema_, references_, value);
            case 'Never':
                return yield* FromNever(schema_, references_, value);
            case 'Not':
                return yield* FromNot(schema_, references_, value);
            case 'Null':
                return yield* FromNull(schema_, references_, value);
            case 'Number':
                return yield* FromNumber(schema_, references_, value);
            case 'Object':
                return yield* FromObject(schema_, references_, value);
            case 'Promise':
                return yield* FromPromise(schema_, references_, value);
            case 'Record':
                return yield* FromRecord(schema_, references_, value);
            case 'Ref':
                return yield* FromRef(schema_, references_, value);
            case 'RegExp':
                return yield* FromRegExp(schema_, references_, value);
            case 'String':
                return yield* FromString(schema_, references_, value);
            case 'Symbol':
                return yield* FromSymbol(schema_, references_, value);
            case 'TemplateLiteral':
                return yield* FromTemplateLiteral(schema_, references_, value);
            case 'This':
                return yield* FromThis(schema_, references_, value);
            case 'Tuple':
                return yield* FromTuple(schema_, references_, value);
            case 'Undefined':
                return yield* FromUndefined(schema_, references_, value);
            case 'Union':
                return yield* FromUnion(schema_, references_, value);
            case 'Uint8Array':
                return yield* FromUint8Array(schema_, references_, value);
            case 'Unknown':
                return yield* FromUnknown(schema_, references_, value);
            case 'Void':
                return yield* FromVoid(schema_, references_, value);
            default:
                if (!TypeRegistry.Has(schema_[Kind]))
                    throw new TypeCompilerUnknownTypeError(schema);
                return yield* FromKind(schema_, references_, value);
        }
    }
    // ----------------------------------------------------------------
    // Compiler State
    // ----------------------------------------------------------------
    // prettier-ignore
    const state = {
        language: 'javascript', // target language
        functions: new Map(), // local functions
        variables: new Map(), // local variables
        instances: new Map() // exterior kind instances
    };
    // ----------------------------------------------------------------
    // Compiler Factory
    // ----------------------------------------------------------------
    function CreateExpression(schema, references, value, useHoisting = true) {
        return `(${[...Visit(schema, references, value, useHoisting)].join(' && ')})`;
    }
    function CreateFunctionName($id) {
        return `check_${Identifier.Encode($id)}`;
    }
    function CreateVariable(expression) {
        const variableName = `local_${state.variables.size}`;
        state.variables.set(variableName, `const ${variableName} = ${expression}`);
        return variableName;
    }
    function CreateFunction(name, schema, references, value, useHoisting = true) {
        const [newline, pad] = ['\n', (length) => ''.padStart(length, ' ')];
        const parameter = CreateParameter('value', 'any');
        const returns = CreateReturns('boolean');
        const expression = [...Visit(schema, references, value, useHoisting)].map((expression) => `${pad(4)}${expression}`).join(` &&${newline}`);
        return `function ${name}(${parameter})${returns} {${newline}${pad(2)}return (${newline}${expression}${newline}${pad(2)})\n}`;
    }
    function CreateParameter(name, type) {
        const annotation = state.language === 'typescript' ? `: ${type}` : '';
        return `${name}${annotation}`;
    }
    function CreateReturns(type) {
        return state.language === 'typescript' ? `: ${type}` : '';
    }
    // ----------------------------------------------------------------
    // Compile
    // ----------------------------------------------------------------
    function Build(schema, references, options) {
        const functionCode = CreateFunction('check', schema, references, 'value'); // will populate functions and variables
        const parameter = CreateParameter('value', 'any');
        const returns = CreateReturns('boolean');
        const functions = [...state.functions.values()];
        const variables = [...state.variables.values()];
        // prettier-ignore
        const checkFunction = IsString(schema.$id) // ensure top level schemas with $id's are hoisted
            ? `return function check(${parameter})${returns} {\n  return ${CreateFunctionName(schema.$id)}(value)\n}`
            : `return ${functionCode}`;
        return [...variables, ...functions, checkFunction].join('\n');
    }
    /** Generates the code used to assert this type and returns it as a string */
    function Code(...args) {
        const defaults = { language: 'javascript' };
        // prettier-ignore
        const [schema, references, options] = (args.length === 2 && IsArray(args[1]) ? [args[0], args[1], defaults] :
            args.length === 2 && !IsArray(args[1]) ? [args[0], [], args[1]] :
                args.length === 3 ? [args[0], args[1], args[2]] :
                    args.length === 1 ? [args[0], [], defaults] :
                        [null, [], defaults]);
        // compiler-reset
        state.language = options.language;
        state.variables.clear();
        state.functions.clear();
        state.instances.clear();
        if (!IsSchema(schema))
            throw new TypeCompilerTypeGuardError(schema);
        for (const schema of references)
            if (!IsSchema(schema))
                throw new TypeCompilerTypeGuardError(schema);
        return Build(schema, references, options);
    }
    TypeCompiler.Code = Code;
    /** Compiles a TypeBox type for optimal runtime type checking. Types must be valid TypeBox types of TSchema */
    function Compile(schema, references = []) {
        const generatedCode = Code(schema, references, { language: 'javascript' });
        const compiledFunction = Evaluate('kind', 'format', 'hash', generatedCode);
        const instances = new Map(state.instances);
        function typeRegistryFunction(kind, instance, value) {
            if (!TypeRegistry.Has(kind) || !instances.has(instance))
                return false;
            const checkFunc = TypeRegistry.Get(kind);
            const schema = instances.get(instance);
            return checkFunc(schema, value);
        }
        function formatRegistryFunction(format, value) {
            if (!FormatRegistry.Has(format))
                return false;
            const checkFunc = FormatRegistry.Get(format);
            return checkFunc(value);
        }
        function hashFunction(value) {
            return Hash(value);
        }
        const checkFunction = compiledFunction(typeRegistryFunction, formatRegistryFunction, hashFunction);
        return new TypeCheck(schema, references, checkFunction, generatedCode);
    }
    TypeCompiler.Compile = Compile;
})(TypeCompiler || (TypeCompiler = {}));
