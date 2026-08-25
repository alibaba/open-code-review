/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

export declare class TestPathPatterns {
  readonly patterns: Array<string>;
  constructor(patterns: Array<string>);
  /**
   * Return true if there are any patterns.
   */
  isSet(): boolean;
  /**
   * Return true if the patterns are valid.
   */
  isValid(): boolean;
  /**
   * Return a human-friendly version of the pattern regex.
   */
  toPretty(): string;
  /**
   * Return a TestPathPatternsExecutor that can execute the patterns.
   */
  toExecutor(
    options: TestPathPatternsExecutorOptions,
  ): TestPathPatternsExecutor;
  /** For jest serializers */
  toJSON(): any;
}

export declare class TestPathPatternsExecutor {
  readonly patterns: TestPathPatterns;
  private readonly options;
  constructor(
    patterns: TestPathPatterns,
    options: TestPathPatternsExecutorOptions,
  );
  private toRegex;
  /**
   * Return true if there are any patterns.
   */
  isSet(): boolean;
  /**
   * Return true if the patterns are valid.
   */
  isValid(): boolean;
  /**
   * Return true if the given ABSOLUTE path matches the patterns.
   *
   * Throws an error if the patterns form an invalid regex (see `validate`).
   */
  isMatch(absPath: string): boolean;
  /**
   * Return a human-friendly version of the pattern regex.
   */
  toPretty(): string;
}

export declare type TestPathPatternsExecutorOptions = {
  rootDir: string;
};
