import type { Distribution } from '../types/Distribution.js';
import type { RandomGenerator } from '../types/RandomGenerator.js';
declare function uniformIntDistribution(from: number, to: number): Distribution<number>;
declare function uniformIntDistribution(from: number, to: number, rng: RandomGenerator): [number, RandomGenerator];
export { uniformIntDistribution };
