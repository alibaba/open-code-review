import type { RandomGenerator } from './RandomGenerator.js';
export type Distribution<T> = (rng: RandomGenerator) => [T, RandomGenerator];
