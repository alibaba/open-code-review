export type ArrayInt = {
    sign: -1 | 1;
    data: number[];
};
export declare function addArrayIntToNew(arrayIntA: ArrayInt, arrayIntB: ArrayInt): ArrayInt;
export declare function addOneToPositiveArrayInt(arrayInt: ArrayInt): ArrayInt;
export declare function substractArrayIntToNew(arrayIntA: ArrayInt, arrayIntB: ArrayInt): ArrayInt;
export declare function trimArrayIntInplace(arrayInt: ArrayInt): ArrayInt;
