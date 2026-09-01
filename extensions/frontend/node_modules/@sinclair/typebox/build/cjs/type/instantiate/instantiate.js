"use strict";

var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.FromTypes = FromTypes;
exports.Instantiate = Instantiate;
const type_1 = require("../clone/type");
const index_1 = require("../unknown/index");
const index_2 = require("../readonly-optional/index");
const index_3 = require("../readonly/index");
const index_4 = require("../optional/index");
const index_5 = require("../object/index");
const index_6 = require("../record/index");
const ValueGuard = __importStar(require("../guard/value"));
const KindGuard = __importStar(require("../guard/kind"));
// prettier-ignore
function FromConstructor(args, type) {
    type.parameters = FromTypes(args, type.parameters);
    type.returns = FromType(args, type.returns);
    return type;
}
// prettier-ignore
function FromFunction(args, type) {
    type.parameters = FromTypes(args, type.parameters);
    type.returns = FromType(args, type.returns);
    return type;
}
// prettier-ignore
function FromIntersect(args, type) {
    type.allOf = FromTypes(args, type.allOf);
    return type;
}
// prettier-ignore
function FromUnion(args, type) {
    type.anyOf = FromTypes(args, type.anyOf);
    return type;
}
// prettier-ignore
function FromTuple(args, type) {
    if (ValueGuard.IsUndefined(type.items))
        return type;
    type.items = FromTypes(args, type.items);
    return type;
}
// prettier-ignore
function FromArray(args, type) {
    type.items = FromType(args, type.items);
    return type;
}
// prettier-ignore
function FromAsyncIterator(args, type) {
    type.items = FromType(args, type.items);
    return type;
}
// prettier-ignore
function FromIterator(args, type) {
    type.items = FromType(args, type.items);
    return type;
}
// prettier-ignore
function FromPromise(args, type) {
    type.item = FromType(args, type.item);
    return type;
}
// prettier-ignore
function FromObject(args, type) {
    const mappedProperties = FromProperties(args, type.properties);
    return { ...type, ...(0, index_5.Object)(mappedProperties) }; // retain options
}
// prettier-ignore
function FromRecord(args, type) {
    const mappedKey = FromType(args, (0, index_6.RecordKey)(type));
    const mappedValue = FromType(args, (0, index_6.RecordValue)(type));
    const result = (0, index_6.Record)(mappedKey, mappedValue);
    return { ...type, ...result }; // retain options
}
// prettier-ignore
function FromArgument(args, argument) {
    return argument.index in args ? args[argument.index] : (0, index_1.Unknown)();
}
// prettier-ignore
function FromProperty(args, type) {
    const isReadonly = KindGuard.IsReadonly(type);
    const isOptional = KindGuard.IsOptional(type);
    const mapped = FromType(args, type);
    return (isReadonly && isOptional ? (0, index_2.ReadonlyOptional)(mapped) :
        isReadonly && !isOptional ? (0, index_3.Readonly)(mapped) :
            !isReadonly && isOptional ? (0, index_4.Optional)(mapped) :
                mapped);
}
// prettier-ignore
function FromProperties(args, properties) {
    return globalThis.Object.getOwnPropertyNames(properties).reduce((result, key) => {
        return { ...result, [key]: FromProperty(args, properties[key]) };
    }, {});
}
// prettier-ignore
function FromTypes(args, types) {
    return types.map(type => FromType(args, type));
}
// prettier-ignore
function FromType(args, type) {
    return (KindGuard.IsConstructor(type) ? FromConstructor(args, type) :
        KindGuard.IsFunction(type) ? FromFunction(args, type) :
            KindGuard.IsIntersect(type) ? FromIntersect(args, type) :
                KindGuard.IsUnion(type) ? FromUnion(args, type) :
                    KindGuard.IsTuple(type) ? FromTuple(args, type) :
                        KindGuard.IsArray(type) ? FromArray(args, type) :
                            KindGuard.IsAsyncIterator(type) ? FromAsyncIterator(args, type) :
                                KindGuard.IsIterator(type) ? FromIterator(args, type) :
                                    KindGuard.IsPromise(type) ? FromPromise(args, type) :
                                        KindGuard.IsObject(type) ? FromObject(args, type) :
                                            KindGuard.IsRecord(type) ? FromRecord(args, type) :
                                                KindGuard.IsArgument(type) ? FromArgument(args, type) :
                                                    type);
}
/** `[JavaScript]` Instantiates a type with the given parameters */
// prettier-ignore
function Instantiate(type, args) {
    return FromType(args, (0, type_1.CloneType)(type));
}
