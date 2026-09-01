import type { ArrayInt } from './ArrayInt.js';
export type ArrayInt64 = Required<ArrayInt> & {
    data: [number, number];
};
export declare function fromNumberToArrayInt64(out: ArrayInt64, n: number): ArrayInt64;
export declare function substractArrayInt64(out: ArrayInt64, arrayIntA: ArrayInt64, arrayIntB: ArrayInt64): ArrayInt64;
