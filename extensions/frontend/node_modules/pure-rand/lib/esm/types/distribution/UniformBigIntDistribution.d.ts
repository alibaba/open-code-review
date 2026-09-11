import type { Distribution } from '../types/Distribution.js';
import type { RandomGenerator } from '../types/RandomGenerator.js';
declare function uniformBigIntDistribution(from: bigint, to: bigint): Distribution<bigint>;
declare function uniformBigIntDistribution(from: bigint, to: bigint, rng: RandomGenerator): [bigint, RandomGenerator];
export { uniformBigIntDistribution };
