// ------------------------------------------------------------------
// Value Guard
// ------------------------------------------------------------------
// prettier-ignore
function HasPropertyKey(value, key) {
    return key in value;
}
// prettier-ignore
function IsObjectValue(value) {
    return typeof value === 'object' && value !== null;
}
// prettier-ignore
function IsArrayValue(value) {
    return globalThis.Array.isArray(value);
}
// ------------------------------------------------------------------
// Parser Guard
// ------------------------------------------------------------------
/** Returns true if the value is a Array Parser */
export function IsArray(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Array' && HasPropertyKey(value, 'parser') && IsObjectValue(value.parser);
}
/** Returns true if the value is a Const Parser */
export function IsConst(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Const' && HasPropertyKey(value, 'value') && typeof value.value === 'string';
}
/** Returns true if the value is a Context Parser */
export function IsContext(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Context' && HasPropertyKey(value, 'left') && IsParser(value.left) && HasPropertyKey(value, 'right') && IsParser(value.right);
}
/** Returns true if the value is a Ident Parser */
export function IsIdent(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Ident';
}
/** Returns true if the value is a Number Parser */
export function IsNumber(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Number';
}
/** Returns true if the value is a Optional Parser */
export function IsOptional(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Optional' && HasPropertyKey(value, 'parser') && IsObjectValue(value.parser);
}
/** Returns true if the value is a Ref Parser */
export function IsRef(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Ref' && HasPropertyKey(value, 'ref') && typeof value.ref === 'string';
}
/** Returns true if the value is a String Parser */
export function IsString(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'String' && HasPropertyKey(value, 'options') && IsArrayValue(value.options);
}
/** Returns true if the value is a Tuple Parser */
export function IsTuple(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Tuple' && HasPropertyKey(value, 'parsers') && IsArrayValue(value.parsers);
}
/** Returns true if the value is a Union Parser */
export function IsUnion(value) {
    return IsObjectValue(value) && HasPropertyKey(value, 'type') && value.type === 'Union' && HasPropertyKey(value, 'parsers') && IsArrayValue(value.parsers);
}
/** Returns true if the value is a Parser */
export function IsParser(value) {
    // prettier-ignore
    return (IsArray(value) ||
        IsConst(value) ||
        IsContext(value) ||
        IsIdent(value) ||
        IsNumber(value) ||
        IsOptional(value) ||
        IsRef(value) ||
        IsString(value) ||
        IsTuple(value) ||
        IsUnion(value));
}
