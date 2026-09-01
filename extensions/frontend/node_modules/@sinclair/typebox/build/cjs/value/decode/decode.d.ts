import type { TSchema } from '../../type/schema/index';
import type { StaticDecode } from '../../type/static/index';
/** Decodes a value or throws if error */
export declare function Decode<T extends TSchema, Static = StaticDecode<T>, Result extends Static = Static>(schema: T, references: TSchema[], value: unknown): Result;
/** Decodes a value or throws if error */
export declare function Decode<T extends TSchema, Static = StaticDecode<T>, Result extends Static = Static>(schema: T, value: unknown): Result;
