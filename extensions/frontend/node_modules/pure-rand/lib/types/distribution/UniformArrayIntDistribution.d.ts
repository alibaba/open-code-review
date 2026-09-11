import type { Distribution } from '../types/Distribution.js';
import type { RandomGenerator } from '../types/RandomGenerator.js';
import type { ArrayInt } from './internals/ArrayInt.js';
declare function uniformArrayIntDistribution(from: ArrayInt, to: ArrayInt): Distribution<ArrayInt>;
declare function uniformArrayIntDistribution(from: ArrayInt, to: ArrayInt, rng: RandomGenerator): [ArrayInt, RandomGenerator];
export { uniformArrayIntDistribution };
