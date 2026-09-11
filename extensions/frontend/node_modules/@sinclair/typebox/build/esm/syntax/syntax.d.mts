import * as t from '../type/index.mjs';
import { TType } from './parser.mjs';
/** `[Experimental]` Parses type expressions into TypeBox types but does not infer */
export declare function NoInfer<Context extends Record<PropertyKey, t.TSchema>, Input extends string>(context: Context, input: Input, options?: t.SchemaOptions): t.TSchema;
/** `[Experimental]` Parses type expressions into TypeBox types but does not infer */
export declare function NoInfer<Input extends string>(input: Input, options?: t.SchemaOptions): t.TSchema;
/** `[Experimental]` Parses type expressions into TypeBox types */
export type TSyntax<Context extends Record<PropertyKey, t.TSchema>, Code extends string> = (TType<Code, Context> extends [infer Type extends t.TSchema, string] ? Type : t.TNever);
/** `[Experimental]` Parses type expressions into TypeBox types */
export declare function Syntax<Context extends Record<PropertyKey, t.TSchema>, Input extends string>(context: Context, input: Input, options?: t.SchemaOptions): TSyntax<Context, Input>;
/** `[Experimental]` Parses type expressions into TypeBox types */
export declare function Syntax<Input extends string>(annotation: Input, options?: t.SchemaOptions): TSyntax<{}, Input>;
