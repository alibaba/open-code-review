import type { RandomGenerator } from '../types/RandomGenerator.js';
declare function fromState(state: readonly number[]): RandomGenerator;
export declare const xoroshiro128plus: ((seed: number) => RandomGenerator) & {
    fromState: typeof fromState;
};
export {};
