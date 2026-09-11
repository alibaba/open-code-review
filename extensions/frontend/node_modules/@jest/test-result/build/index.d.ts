/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {V8Coverage} from 'collect-v8-coverage';
import {CoverageMap, CoverageMapData} from 'istanbul-lib-coverage';
import {ConsoleBuffer} from '@jest/console';
import {
  Circus,
  Config,
  TestResult as TestResult_2,
  TransformTypes,
} from '@jest/types';
import {IHasteFS, IModuleMap} from 'jest-haste-map';
import Resolver from 'jest-resolve';

export declare const addResult: (
  aggregatedResults: AggregatedResult,
  testResult: TestResult,
) => void;

export declare type AggregatedResult = AggregatedResultWithoutCoverage & {
  coverageMap?: CoverageMap | null;
};

declare type AggregatedResultWithoutCoverage = {
  numFailedTests: number;
  numFailedTestSuites: number;
  numPassedTests: number;
  numPassedTestSuites: number;
  numPendingTests: number;
  numTodoTests: number;
  numPendingTestSuites: number;
  numRuntimeErrorTestSuites: number;
  numTotalTests: number;
  numTotalTestSuites: number;
  openHandles: Array<Error>;
  snapshot: SnapshotSummary;
  startTime: number;
  success: boolean;
  testResults: Array<TestResult>;
  wasInterrupted: boolean;
  runExecError?: SerializableError;
};

export declare type AssertionLocation = {
  fullName: string;
  path: string;
};

export declare type AssertionResult = TestResult_2.AssertionResult;

export declare const buildFailureTestResult: (
  testPath: string,
  err: SerializableError,
) => TestResult;

declare type CodeCoverageFormatter = (
  coverage: CoverageMapData | null | undefined,
  reporter: CodeCoverageReporter,
) => Record<string, unknown> | null | undefined;

declare type CodeCoverageReporter = unknown;

export declare const createEmptyTestResult: () => TestResult;

export declare type FailedAssertion = {
  matcherName?: string;
  message?: string;
  actual?: unknown;
  pass?: boolean;
  passed?: boolean;
  expected?: unknown;
  isNot?: boolean;
  stack?: string;
  error?: unknown;
};

declare type FormattedAssertionResult = Pick<
  AssertionResult,
  'ancestorTitles' | 'fullName' | 'location' | 'status' | 'title' | 'duration'
> & {
  failureMessages: AssertionResult['failureMessages'] | null;
};

declare type FormattedTestResult = {
  message: string;
  name: string;
  summary: string;
  status: 'failed' | 'passed' | 'skipped' | 'focused';
  startTime: number;
  endTime: number;
  coverage: unknown;
  assertionResults: Array<FormattedAssertionResult>;
};

export declare type FormattedTestResults = {
  coverageMap?: CoverageMap | null | undefined;
  numFailedTests: number;
  numFailedTestSuites: number;
  numPassedTests: number;
  numPassedTestSuites: number;
  numPendingTests: number;
  numPendingTestSuites: number;
  numRuntimeErrorTestSuites: number;
  numTotalTests: number;
  numTotalTestSuites: number;
  snapshot: SnapshotSummary;
  startTime: number;
  success: boolean;
  testResults: Array<FormattedTestResult>;
  wasInterrupted: boolean;
};

export declare function formatTestResults(
  results: AggregatedResult,
  codeCoverageFormatter?: CodeCoverageFormatter,
  reporter?: CodeCoverageReporter,
): FormattedTestResults;

export declare const makeEmptyAggregatedTestResult: () => AggregatedResult;

export declare type RuntimeTransformResult = TransformTypes.TransformResult;

export declare type SerializableError = TestResult_2.SerializableError;

export declare type SnapshotSummary = {
  added: number;
  didUpdate: boolean;
  failure: boolean;
  filesAdded: number;
  filesRemoved: number;
  filesRemovedList: Array<string>;
  filesUnmatched: number;
  filesUpdated: number;
  matched: number;
  total: number;
  unchecked: number;
  uncheckedKeysByFile: Array<UncheckedSnapshot>;
  unmatched: number;
  updated: number;
};

export declare type Status = AssertionResult['status'];

export declare type Suite = {
  title: string;
  suites: Array<Suite>;
  tests: Array<AssertionResult>;
};

export declare type Test = {
  context: TestContext;
  duration?: number;
  path: string;
};

export declare type TestCaseResult = AssertionResult & {
  startedAt?: number | null;
};

export declare type TestContext = {
  config: Config.ProjectConfig;
  hasteFS: IHasteFS;
  moduleMap: IModuleMap;
  resolver: Resolver;
};

export declare type TestEvents = {
  'test-file-start': [Test];
  'test-file-success': [Test, TestResult];
  'test-file-failure': [Test, SerializableError];
  'test-case-start': [string, Circus.TestCaseStartInfo];
  'test-case-result': [string, TestCaseResult];
};

export declare type TestFileEvent<
  T extends keyof TestEvents = keyof TestEvents,
> = (eventName: T, args: TestEvents[T]) => unknown;

export declare type TestResult = {
  console?: ConsoleBuffer;
  coverage?: CoverageMapData;
  displayName?: Config.DisplayName;
  /**
   * Whether [`test.failing()`](https://jestjs.io/docs/api#testfailingname-fn-timeout)
   * was used.
   */
  failing?: boolean;
  failureMessage?: string | null;
  leaks: boolean;
  memoryUsage?: number;
  numFailingTests: number;
  numPassingTests: number;
  numPendingTests: number;
  numTodoTests: number;
  openHandles: Array<Error>;
  perfStats: {
    end: number;
    loadTestEnvironmentEnd: number;
    loadTestEnvironmentStart: number;
    runtime: number;
    setupAfterEnvEnd: number;
    setupAfterEnvStart: number;
    setupFilesEnd: number;
    setupFilesStart: number;
    slow: boolean;
    start: number;
  };
  skipped: boolean;
  snapshot: {
    added: number;
    fileDeleted: boolean;
    matched: number;
    unchecked: number;
    uncheckedKeys: Array<string>;
    unmatched: number;
    updated: number;
  };
  testExecError?: SerializableError;
  testFilePath: string;
  testResults: Array<AssertionResult>;
  v8Coverage?: V8CoverageResult;
};

export declare type TestResultsProcessor = (
  results: AggregatedResult,
) => AggregatedResult | Promise<AggregatedResult>;

declare type UncheckedSnapshot = {
  filePath: string;
  keys: Array<string>;
};

export declare type V8CoverageResult = Array<{
  codeTransformResult: RuntimeTransformResult | undefined;
  result: V8Coverage[number];
}>;
