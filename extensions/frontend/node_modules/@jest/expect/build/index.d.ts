/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {
  AsymmetricMatchers,
  BaseExpect,
  Inverse,
  MatcherContext,
  MatcherFunction,
  MatcherFunctionWithContext,
  MatcherState,
  MatcherUtils,
  Matchers,
} from 'expect';
import {SnapshotMatchers, addSerializer} from 'jest-snapshot';

export {AsymmetricMatchers};

export declare type JestExpect = {
  <T = unknown>(
    actual: T,
  ): JestMatchers<void, T> &
    Inverse<JestMatchers<void, T>> &
    PromiseMatchers<T>;
  addSnapshotSerializer: typeof addSerializer;
} & BaseExpect &
  AsymmetricMatchers &
  Inverse<Omit<AsymmetricMatchers, 'any' | 'anything'>>;

export declare const jestExpect: JestExpect;

declare type JestMatchers<R extends void | Promise<void>, T> = Matchers<R, T> &
  SnapshotMatchers<R, T>;

export {MatcherContext};

export {MatcherFunction};

export {MatcherFunctionWithContext};

export {Matchers};

export {MatcherState};

export {MatcherUtils};

declare type PromiseMatchers<T = unknown> = {
  /**
   * Unwraps the reason of a rejected promise so any other matcher can be chained.
   * If the promise is fulfilled the assertion fails.
   */
  rejects: JestMatchers<Promise<void>, T> &
    Inverse<JestMatchers<Promise<void>, T>>;
  /**
   * Unwraps the value of a fulfilled promise so any other matcher can be chained.
   * If the promise is rejected the assertion fails.
   */
  resolves: JestMatchers<Promise<void>, T> &
    Inverse<JestMatchers<Promise<void>, T>>;
};
