/** Maps input to output. This is the default Mapping */
export const Identity = (value) => value;
/** Maps the output as the given parameter T */
// prettier-ignore
export const As = (mapping) => (_) => mapping;
/** `[Context]` Creates a Context Parser */
export function Context(...args) {
    const [left, right, mapping] = args.length === 3 ? [args[0], args[1], args[2]] : [args[0], args[1], Identity];
    return { type: 'Context', left, right, mapping };
}
/** `[EBNF]` Creates an Array Parser */
export function Array(...args) {
    const [parser, mapping] = args.length === 2 ? [args[0], args[1]] : [args[0], Identity];
    return { type: 'Array', parser, mapping };
}
/** `[TERM]` Creates a Const Parser */
export function Const(...args) {
    const [value, mapping] = args.length === 2 ? [args[0], args[1]] : [args[0], Identity];
    return { type: 'Const', value, mapping };
}
/** `[BNF]` Creates a Ref Parser. This Parser can only be used in the context of a Module */
export function Ref(...args) {
    const [ref, mapping] = args.length === 2 ? [args[0], args[1]] : [args[0], Identity];
    return { type: 'Ref', ref, mapping };
}
/** `[TERM]` Creates a String Parser. Options are an array of permissable quote characters */
export function String(...params) {
    const [options, mapping] = params.length === 2 ? [params[0], params[1]] : [params[0], Identity];
    return { type: 'String', options, mapping };
}
/** `[TERM]` Creates an Ident Parser where Ident matches any valid JavaScript identifier */
export function Ident(...params) {
    const mapping = params.length === 1 ? params[0] : Identity;
    return { type: 'Ident', mapping };
}
/** `[TERM]` Creates an Number Parser */
export function Number(...params) {
    const mapping = params.length === 1 ? params[0] : Identity;
    return { type: 'Number', mapping };
}
/** `[EBNF]` Creates an Optional Parser */
export function Optional(...args) {
    const [parser, mapping] = args.length === 2 ? [args[0], args[1]] : [args[0], Identity];
    return { type: 'Optional', parser, mapping };
}
/** `[BNF]` Creates a Tuple Parser */
export function Tuple(...args) {
    const [parsers, mapping] = args.length === 2 ? [args[0], args[1]] : [args[0], Identity];
    return { type: 'Tuple', parsers, mapping };
}
/** `[BNF]` Creates a Union parser */
export function Union(...args) {
    const [parsers, mapping] = args.length === 2 ? [args[0], args[1]] : [args[0], Identity];
    return { type: 'Union', parsers, mapping };
}
