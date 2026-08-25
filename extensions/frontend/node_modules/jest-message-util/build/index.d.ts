/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {StackData} from 'stack-utils';
import {Config, TestResult} from '@jest/types';

export declare const formatExecError: (
  error: Error | TestResult.SerializableError | string | number | undefined,
  config: StackTraceConfig,
  options: StackTraceOptions,
  testPath?: string,
  reuseMessage?: boolean,
  noTitle?: boolean,
) => string;

export declare const formatPath: (
  line: string,
  config: StackTraceConfig,
  relativeTestPath?: string | null,
) => string;

export declare const formatResultsErrors: (
  testResults: Array<TestResult.AssertionResult>,
  config: StackTraceConfig,
  options: StackTraceOptions,
  testPath?: string,
) => string | null;

export declare function formatStackTrace(
  stack: string,
  config: StackTraceConfig,
  options: StackTraceOptions,
  testPath?: string,
): string;

export declare interface Frame extends StackData {
  file: string;
}

export declare function getStackTraceLines(
  stack: string,
  options?: StackTraceOptions,
): Array<string>;

export declare function getTopFrame(lines: Array<string>): Frame | null;

export declare const indentAllLines: (lines: string) => string;

export declare const separateMessageFromStack: (content: string) => {
  message: string;
  stack: string;
};

export declare type StackTraceConfig = Pick<
  Config.ProjectConfig,
  'rootDir' | 'testMatch'
>;

export declare type StackTraceOptions = {
  noStackTrace: boolean;
  noCodeFrame?: boolean;
};
