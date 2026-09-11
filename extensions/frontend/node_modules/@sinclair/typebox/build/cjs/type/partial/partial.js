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
exports.Partial = Partial;
const type_1 = require("../create/type");
const index_1 = require("../computed/index");
const index_2 = require("../optional/index");
const index_3 = require("../object/index");
const index_4 = require("../intersect/index");
const index_5 = require("../union/index");
const index_6 = require("../ref/index");
const index_7 = require("../discard/index");
const index_8 = require("../symbols/index");
const partial_from_mapped_result_1 = require("./partial-from-mapped-result");
// ------------------------------------------------------------------
// KindGuard
// ------------------------------------------------------------------
const KindGuard = __importStar(require("../guard/kind"));
// prettier-ignore
function FromComputed(target, parameters) {
    return (0, index_1.Computed)('Partial', [(0, index_1.Computed)(target, parameters)]);
}
// prettier-ignore
function FromRef($ref) {
    return (0, index_1.Computed)('Partial', [(0, index_6.Ref)($ref)]);
}
// prettier-ignore
function FromProperties(properties) {
    const partialProperties = {};
    for (const K of globalThis.Object.getOwnPropertyNames(properties))
        partialProperties[K] = (0, index_2.Optional)(properties[K]);
    return partialProperties;
}
// prettier-ignore
function FromObject(type, properties) {
    const options = (0, index_7.Discard)(type, [index_8.TransformKind, '$id', 'required', 'properties']);
    const mappedProperties = FromProperties(properties);
    return (0, index_3.Object)(mappedProperties, options);
}
// prettier-ignore
function FromRest(types) {
    return types.map(type => PartialResolve(type));
}
// ------------------------------------------------------------------
// PartialResolve
// ------------------------------------------------------------------
// prettier-ignore
function PartialResolve(type) {
    return (
    // Mappable
    KindGuard.IsComputed(type) ? FromComputed(type.target, type.parameters) :
        KindGuard.IsRef(type) ? FromRef(type.$ref) :
            KindGuard.IsIntersect(type) ? (0, index_4.Intersect)(FromRest(type.allOf)) :
                KindGuard.IsUnion(type) ? (0, index_5.Union)(FromRest(type.anyOf)) :
                    KindGuard.IsObject(type) ? FromObject(type, type.properties) :
                        // Intrinsic
                        KindGuard.IsBigInt(type) ? type :
                            KindGuard.IsBoolean(type) ? type :
                                KindGuard.IsInteger(type) ? type :
                                    KindGuard.IsLiteral(type) ? type :
                                        KindGuard.IsNull(type) ? type :
                                            KindGuard.IsNumber(type) ? type :
                                                KindGuard.IsString(type) ? type :
                                                    KindGuard.IsSymbol(type) ? type :
                                                        KindGuard.IsUndefined(type) ? type :
                                                            // Passthrough
                                                            (0, index_3.Object)({}));
}
/** `[Json]` Constructs a type where all properties are optional */
function Partial(type, options) {
    if (KindGuard.IsMappedResult(type)) {
        return (0, partial_from_mapped_result_1.PartialFromMappedResult)(type, options);
    }
    else {
        // special: mapping types require overridable options
        return (0, type_1.CreateType)({ ...PartialResolve(type), ...options });
    }
}
