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
exports.FromType = FromType;
exports.ComputeType = ComputeType;
exports.ComputeModuleProperties = ComputeModuleProperties;
const index_1 = require("../create/index");
const index_2 = require("../clone/index");
const index_3 = require("../discard/index");
const index_4 = require("../array/index");
const index_5 = require("../awaited/index");
const index_6 = require("../async-iterator/index");
const index_7 = require("../constructor/index");
const index_8 = require("../indexed/index");
const index_9 = require("../function/index");
const index_10 = require("../intersect/index");
const index_11 = require("../iterator/index");
const index_12 = require("../keyof/index");
const index_13 = require("../object/index");
const index_14 = require("../omit/index");
const index_15 = require("../pick/index");
const index_16 = require("../never/index");
const index_17 = require("../partial/index");
const index_18 = require("../record/index");
const index_19 = require("../required/index");
const index_20 = require("../tuple/index");
const index_21 = require("../union/index");
// ------------------------------------------------------------------
// Symbols
// ------------------------------------------------------------------
const index_22 = require("../symbols/index");
// ------------------------------------------------------------------
// KindGuard
// ------------------------------------------------------------------
const KindGuard = __importStar(require("../guard/kind"));
// prettier-ignore
function DereferenceParameters(moduleProperties, types) {
    return types.map((type) => {
        return KindGuard.IsRef(type)
            ? Dereference(moduleProperties, type.$ref)
            : FromType(moduleProperties, type);
    });
}
// prettier-ignore
function Dereference(moduleProperties, ref) {
    return (ref in moduleProperties
        ? KindGuard.IsRef(moduleProperties[ref])
            ? Dereference(moduleProperties, moduleProperties[ref].$ref)
            : FromType(moduleProperties, moduleProperties[ref])
        : (0, index_16.Never)());
}
// prettier-ignore
function FromAwaited(parameters) {
    return (0, index_5.Awaited)(parameters[0]);
}
// prettier-ignore
function FromIndex(parameters) {
    return (0, index_8.Index)(parameters[0], parameters[1]);
}
// prettier-ignore
function FromKeyOf(parameters) {
    return (0, index_12.KeyOf)(parameters[0]);
}
// prettier-ignore
function FromPartial(parameters) {
    return (0, index_17.Partial)(parameters[0]);
}
// prettier-ignore
function FromOmit(parameters) {
    return (0, index_14.Omit)(parameters[0], parameters[1]);
}
// prettier-ignore
function FromPick(parameters) {
    return (0, index_15.Pick)(parameters[0], parameters[1]);
}
// prettier-ignore
function FromRequired(parameters) {
    return (0, index_19.Required)(parameters[0]);
}
// prettier-ignore
function FromComputed(moduleProperties, target, parameters) {
    const dereferenced = DereferenceParameters(moduleProperties, parameters);
    return (target === 'Awaited' ? FromAwaited(dereferenced) :
        target === 'Index' ? FromIndex(dereferenced) :
            target === 'KeyOf' ? FromKeyOf(dereferenced) :
                target === 'Partial' ? FromPartial(dereferenced) :
                    target === 'Omit' ? FromOmit(dereferenced) :
                        target === 'Pick' ? FromPick(dereferenced) :
                            target === 'Required' ? FromRequired(dereferenced) :
                                (0, index_16.Never)());
}
function FromArray(moduleProperties, type) {
    return (0, index_4.Array)(FromType(moduleProperties, type));
}
function FromAsyncIterator(moduleProperties, type) {
    return (0, index_6.AsyncIterator)(FromType(moduleProperties, type));
}
// prettier-ignore
function FromConstructor(moduleProperties, parameters, instanceType) {
    return (0, index_7.Constructor)(FromTypes(moduleProperties, parameters), FromType(moduleProperties, instanceType));
}
// prettier-ignore
function FromFunction(moduleProperties, parameters, returnType) {
    return (0, index_9.Function)(FromTypes(moduleProperties, parameters), FromType(moduleProperties, returnType));
}
function FromIntersect(moduleProperties, types) {
    return (0, index_10.Intersect)(FromTypes(moduleProperties, types));
}
function FromIterator(moduleProperties, type) {
    return (0, index_11.Iterator)(FromType(moduleProperties, type));
}
function FromObject(moduleProperties, properties) {
    return (0, index_13.Object)(globalThis.Object.keys(properties).reduce((result, key) => {
        return { ...result, [key]: FromType(moduleProperties, properties[key]) };
    }, {}));
}
// prettier-ignore
function FromRecord(moduleProperties, type) {
    const [value, pattern] = [FromType(moduleProperties, (0, index_18.RecordValue)(type)), (0, index_18.RecordPattern)(type)];
    const result = (0, index_2.CloneType)(type);
    result.patternProperties[pattern] = value;
    return result;
}
// prettier-ignore
function FromTransform(moduleProperties, transform) {
    return (KindGuard.IsRef(transform))
        ? { ...Dereference(moduleProperties, transform.$ref), [index_22.TransformKind]: transform[index_22.TransformKind] }
        : transform;
}
function FromTuple(moduleProperties, types) {
    return (0, index_20.Tuple)(FromTypes(moduleProperties, types));
}
function FromUnion(moduleProperties, types) {
    return (0, index_21.Union)(FromTypes(moduleProperties, types));
}
function FromTypes(moduleProperties, types) {
    return types.map((type) => FromType(moduleProperties, type));
}
// prettier-ignore
function FromType(moduleProperties, type) {
    return (
    // Modifiers
    KindGuard.IsOptional(type) ? (0, index_1.CreateType)(FromType(moduleProperties, (0, index_3.Discard)(type, [index_22.OptionalKind])), type) :
        KindGuard.IsReadonly(type) ? (0, index_1.CreateType)(FromType(moduleProperties, (0, index_3.Discard)(type, [index_22.ReadonlyKind])), type) :
            // Transform
            KindGuard.IsTransform(type) ? (0, index_1.CreateType)(FromTransform(moduleProperties, type), type) :
                // Types
                KindGuard.IsArray(type) ? (0, index_1.CreateType)(FromArray(moduleProperties, type.items), type) :
                    KindGuard.IsAsyncIterator(type) ? (0, index_1.CreateType)(FromAsyncIterator(moduleProperties, type.items), type) :
                        KindGuard.IsComputed(type) ? (0, index_1.CreateType)(FromComputed(moduleProperties, type.target, type.parameters)) :
                            KindGuard.IsConstructor(type) ? (0, index_1.CreateType)(FromConstructor(moduleProperties, type.parameters, type.returns), type) :
                                KindGuard.IsFunction(type) ? (0, index_1.CreateType)(FromFunction(moduleProperties, type.parameters, type.returns), type) :
                                    KindGuard.IsIntersect(type) ? (0, index_1.CreateType)(FromIntersect(moduleProperties, type.allOf), type) :
                                        KindGuard.IsIterator(type) ? (0, index_1.CreateType)(FromIterator(moduleProperties, type.items), type) :
                                            KindGuard.IsObject(type) ? (0, index_1.CreateType)(FromObject(moduleProperties, type.properties), type) :
                                                KindGuard.IsRecord(type) ? (0, index_1.CreateType)(FromRecord(moduleProperties, type)) :
                                                    KindGuard.IsTuple(type) ? (0, index_1.CreateType)(FromTuple(moduleProperties, type.items || []), type) :
                                                        KindGuard.IsUnion(type) ? (0, index_1.CreateType)(FromUnion(moduleProperties, type.anyOf), type) :
                                                            type);
}
// prettier-ignore
function ComputeType(moduleProperties, key) {
    return (key in moduleProperties
        ? FromType(moduleProperties, moduleProperties[key])
        : (0, index_16.Never)());
}
// prettier-ignore
function ComputeModuleProperties(moduleProperties) {
    return globalThis.Object.getOwnPropertyNames(moduleProperties).reduce((result, key) => {
        return { ...result, [key]: ComputeType(moduleProperties, key) };
    }, {});
}
