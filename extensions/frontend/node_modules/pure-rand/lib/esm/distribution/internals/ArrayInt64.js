export function fromNumberToArrayInt64(out, n) {
    if (n < 0) {
        var posN = -n;
        out.sign = -1;
        out.data[0] = ~~(posN / 0x100000000);
        out.data[1] = posN >>> 0;
    }
    else {
        out.sign = 1;
        out.data[0] = ~~(n / 0x100000000);
        out.data[1] = n >>> 0;
    }
    return out;
}
export function substractArrayInt64(out, arrayIntA, arrayIntB) {
    var lowA = arrayIntA.data[1];
    var highA = arrayIntA.data[0];
    var signA = arrayIntA.sign;
    var lowB = arrayIntB.data[1];
    var highB = arrayIntB.data[0];
    var signB = arrayIntB.sign;
    out.sign = 1;
    if (signA === 1 && signB === -1) {
        var low_1 = lowA + lowB;
        var high = highA + highB + (low_1 > 0xffffffff ? 1 : 0);
        out.data[0] = high >>> 0;
        out.data[1] = low_1 >>> 0;
        return out;
    }
    var lowFirst = lowA;
    var highFirst = highA;
    var lowSecond = lowB;
    var highSecond = highB;
    if (signA === -1) {
        lowFirst = lowB;
        highFirst = highB;
        lowSecond = lowA;
        highSecond = highA;
    }
    var reminderLow = 0;
    var low = lowFirst - lowSecond;
    if (low < 0) {
        reminderLow = 1;
        low = low >>> 0;
    }
    out.data[0] = highFirst - highSecond - reminderLow;
    out.data[1] = low;
    return out;
}
