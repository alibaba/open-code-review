export interface RandomGenerator {
    clone(): RandomGenerator;
    next(): [number, RandomGenerator];
    jump?(): RandomGenerator;
    unsafeNext(): number;
    unsafeJump?(): void;
    getState(): readonly number[];
}
