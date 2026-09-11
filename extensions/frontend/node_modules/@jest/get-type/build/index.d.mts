//#region src/index.d.ts
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
type ValueType = 'array' | 'bigint' | 'boolean' | 'function' | 'null' | 'number' | 'object' | 'regexp' | 'map' | 'set' | 'date' | 'string' | 'symbol' | 'undefined';
declare function getType(value: unknown): ValueType;
declare const isPrimitive: (value: unknown) => boolean;
//#endregion
export { getType, isPrimitive };