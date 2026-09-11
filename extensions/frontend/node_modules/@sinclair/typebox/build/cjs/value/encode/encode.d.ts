import type { TSchema } from '../../type/schema/index';
import type { StaticEncode } from '../../type/static/index';
/** Encodes a value or throws if error */
export declare function Encode<T extends TSchema, Static = StaticEncode<T>, Result extends Static = Static>(schema: T, references: TSchema[], value: unknown): Result;
/** Encodes a value or throws if error */
export declare function Encode<T extends TSchema, Static = StaticEncode<T>, Result extends Static = Static>(schema: T, value: unknown): Result;
