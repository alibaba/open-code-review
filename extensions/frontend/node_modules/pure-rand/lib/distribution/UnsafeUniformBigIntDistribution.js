"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.unsafeUniformBigIntDistribution = unsafeUniformBigIntDistribution;
var SBigInt = typeof BigInt !== 'undefined' ? BigInt : undefined;
var One = typeof BigInt !== 'undefined' ? BigInt(1) : undefined;
var ThirtyTwo = typeof BigInt !== 'undefined' ? BigInt(32) : undefined;
var NumValues = typeof BigInt !== 'undefined' ? BigInt(0x100000000) : undefined;
function unsafeUniformBigIntDistribution(from, to, rng) {
    var diff = to - from + One;
    var FinalNumValues = NumValues;
    var NumIterations = 1;
    while (FinalNumValues < diff) {
        FinalNumValues <<= ThirtyTwo;
        ++NumIterations;
    }
    var value = generateNext(NumIterations, rng);
    if (value < diff) {
        return value + from;
    }
    if (value + diff < FinalNumValues) {
        return (value % diff) + from;
    }
    var MaxAcceptedRandom = FinalNumValues - (FinalNumValues % diff);
    while (value >= MaxAcceptedRandom) {
        value = generateNext(NumIterations, rng);
    }
    return (value % diff) + from;
}
function generateNext(NumIterations, rng) {
    var value = SBigInt(rng.unsafeNext() + 0x80000000);
    for (var num = 1; num < NumIterations; ++num) {
        var out = rng.unsafeNext();
        value = (value << ThirtyTwo) + SBigInt(out + 0x80000000);
    }
    return value;
}
