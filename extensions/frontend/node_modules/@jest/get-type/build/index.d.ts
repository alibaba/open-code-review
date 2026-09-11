/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

export declare function getType(value: unknown): ValueType;

export declare const isPrimitive: (
  value: unknown,
) => value is null | undefined | string | number | boolean | symbol | bigint;

declare type ValueType =
  | 'array'
  | 'bigint'
  | 'boolean'
  | 'function'
  | 'null'
  | 'number'
  | 'object'
  | 'regexp'
  | 'map'
  | 'set'
  | 'date'
  | 'string'
  | 'symbol'
  | 'undefined';

export {};
