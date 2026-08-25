"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.addArrayIntToNew = addArrayIntToNew;
exports.addOneToPositiveArrayInt = addOneToPositiveArrayInt;
exports.substractArrayIntToNew = substractArrayIntToNew;
exports.trimArrayIntInplace = trimArrayIntInplace;
function addArrayIntToNew(arrayIntA, arrayIntB) {
    if (arrayIntA.sign !== arrayIntB.sign) {
        return substractArrayIntToNew(arrayIntA, { sign: -arrayIntB.sign, data: arrayIntB.data });
    }
    var data = [];
    var reminder = 0;
    var dataA = arrayIntA.data;
    var dataB = arrayIntB.data;
    for (var indexA = dataA.length - 1, indexB = dataB.length - 1; indexA >= 0 || indexB >= 0; --indexA, --indexB) {
        var vA = indexA >= 0 ? dataA[indexA] : 0;
        var vB = indexB >= 0 ? dataB[indexB] : 0;
        var current = vA + vB + reminder;
        data.push(current >>> 0);
        reminder = ~~(current / 0x100000000);
    }
    if (reminder !== 0) {
        data.push(reminder);
    }
    return { sign: arrayIntA.sign, data: data.reverse() };
}
function addOneToPositiveArrayInt(arrayInt) {
    arrayInt.sign = 1;
    var data = arrayInt.data;
    for (var index = data.length - 1; index >= 0; --index) {
        if (data[index] === 0xffffffff) {
            data[index] = 0;
        }
        else {
            data[index] += 1;
            return arrayInt;
        }
    }
    data.unshift(1);
    return arrayInt;
}
function isStrictlySmaller(dataA, dataB) {
    var maxLength = Math.max(dataA.length, dataB.length);
    for (var index = 0; index < maxLength; ++index) {
        var indexA = index + dataA.length - maxLength;
        var indexB = index + dataB.length - maxLength;
        var vA = indexA >= 0 ? dataA[indexA] : 0;
        var vB = indexB >= 0 ? dataB[indexB] : 0;
        if (vA < vB)
            return true;
        if (vA > vB)
            return false;
    }
    return false;
}
function substractArrayIntToNew(arrayIntA, arrayIntB) {
    if (arrayIntA.sign !== arrayIntB.sign) {
        return addArrayIntToNew(arrayIntA, { sign: -arrayIntB.sign, data: arrayIntB.data });
    }
    var dataA = arrayIntA.data;
    var dataB = arrayIntB.data;
    if (isStrictlySmaller(dataA, dataB)) {
        var out = substractArrayIntToNew(arrayIntB, arrayIntA);
        out.sign = -out.sign;
        return out;
    }
    var data = [];
    var reminder = 0;
    for (var indexA = dataA.length - 1, indexB = dataB.length - 1; indexA >= 0 || indexB >= 0; --indexA, --indexB) {
        var vA = indexA >= 0 ? dataA[indexA] : 0;
        var vB = indexB >= 0 ? dataB[indexB] : 0;
        var current = vA - vB - reminder;
        data.push(current >>> 0);
        reminder = current < 0 ? 1 : 0;
    }
    return { sign: arrayIntA.sign, data: data.reverse() };
}
function trimArrayIntInplace(arrayInt) {
    var data = arrayInt.data;
    var firstNonZero = 0;
    for (; firstNonZero !== data.length && data[firstNonZero] === 0; ++firstNonZero) { }
    if (firstNonZero === data.length) {
        arrayInt.sign = 1;
        arrayInt.data = [0];
        return arrayInt;
    }
    data.splice(0, firstNonZero);
    return arrayInt;
}
