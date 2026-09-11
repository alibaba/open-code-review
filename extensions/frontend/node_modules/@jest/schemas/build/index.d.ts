/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {
  Static,
  TArray,
  TBoolean,
  TInteger,
  TIntersect,
  TLiteral,
  TNull,
  TNumber,
  TObject,
  TOptional,
  TRecord,
  TString,
  TTuple,
  TUnion,
  TUnknown,
  TUnsafe,
} from '@sinclair/typebox';

export declare const FakeTimers: TIntersect<
  [
    TObject<{
      enableGlobally: TOptional<TBoolean>;
    }>,
    TUnion<
      [
        TObject<{
          advanceTimers: TOptional<TUnion<[TBoolean, TNumber]>>;
          doNotFake: TOptional<
            TArray<
              TUnion<
                [
                  TLiteral<'Date'>,
                  TLiteral<'hrtime'>,
                  TLiteral<'nextTick'>,
                  TLiteral<'performance'>,
                  TLiteral<'queueMicrotask'>,
                  TLiteral<'requestAnimationFrame'>,
                  TLiteral<'cancelAnimationFrame'>,
                  TLiteral<'requestIdleCallback'>,
                  TLiteral<'cancelIdleCallback'>,
                  TLiteral<'setImmediate'>,
                  TLiteral<'clearImmediate'>,
                  TLiteral<'setInterval'>,
                  TLiteral<'clearInterval'>,
                  TLiteral<'setTimeout'>,
                  TLiteral<'clearTimeout'>,
                  TLiteral<'Temporal'>,
                ]
              >
            >
          >;
          now: TOptional<TInteger>;
          timerLimit: TOptional<TNumber>;
          legacyFakeTimers: TOptional<TLiteral<false>>;
        }>,
        TObject<{
          legacyFakeTimers: TOptional<TLiteral<true>>;
        }>,
      ]
    >,
  ]
>;

export declare type FakeTimers = Static<typeof FakeTimers>;

export declare const InitialOptions: TObject<{
  automock: TOptional<TBoolean>;
  bail: TOptional<TUnion<[TBoolean, TNumber]>>;
  cache: TOptional<TBoolean>;
  cacheDirectory: TOptional<TString>;
  ci: TOptional<TBoolean>;
  clearMocks: TOptional<TBoolean>;
  changedFilesWithAncestor: TOptional<TBoolean>;
  changedSince: TOptional<TString>;
  collectCoverage: TOptional<TBoolean>;
  collectCoverageFrom: TOptional<TArray<TString>>;
  collectTests: TOptional<TBoolean>;
  coverageDirectory: TOptional<TString>;
  coveragePathIgnorePatterns: TOptional<TArray<TString>>;
  coverageProvider: TOptional<TUnion<[TLiteral<'babel'>, TLiteral<'v8'>]>>;
  coverageReporters: TOptional<
    TArray<
      TUnion<
        [
          TUnion<
            [
              TLiteral<'clover'>,
              TLiteral<'cobertura'>,
              TLiteral<'html-spa'>,
              TLiteral<'html'>,
              TLiteral<'json'>,
              TLiteral<'json-summary'>,
              TLiteral<'lcov'>,
              TLiteral<'lcovonly'>,
              TLiteral<'none'>,
              TLiteral<'teamcity'>,
              TLiteral<'text'>,
              TLiteral<'text-lcov'>,
              TLiteral<'text-summary'>,
            ]
          >,
          TTuple<
            [
              TUnion<
                [
                  TLiteral<'clover'>,
                  TLiteral<'cobertura'>,
                  TLiteral<'html-spa'>,
                  TLiteral<'html'>,
                  TLiteral<'json'>,
                  TLiteral<'json-summary'>,
                  TLiteral<'lcov'>,
                  TLiteral<'lcovonly'>,
                  TLiteral<'none'>,
                  TLiteral<'teamcity'>,
                  TLiteral<'text'>,
                  TLiteral<'text-lcov'>,
                  TLiteral<'text-summary'>,
                ]
              >,
              TRecord<TString, TUnknown>,
            ]
          >,
        ]
      >
    >
  >;
  coverageThreshold: TOptional<
    TUnsafe<{
      [path: string]: {
        branches?: number | undefined;
        functions?: number | undefined;
        lines?: number | undefined;
        statements?: number | undefined;
      };
      global: Static<
        TObject<{
          branches: TOptional<TNumber>;
          functions: TOptional<TNumber>;
          lines: TOptional<TNumber>;
          statements: TOptional<TNumber>;
        }>
      >;
    }>
  >;
  dependencyExtractor: TOptional<TString>;
  detectLeaks: TOptional<TBoolean>;
  detectOpenHandles: TOptional<TBoolean>;
  displayName: TOptional<
    TUnion<
      [
        TString,
        TObject<{
          name: TString;
          color: TUnion<
            [
              TLiteral<'black'>,
              TLiteral<'red'>,
              TLiteral<'green'>,
              TLiteral<'yellow'>,
              TLiteral<'blue'>,
              TLiteral<'magenta'>,
              TLiteral<'cyan'>,
              TLiteral<'white'>,
              TLiteral<'gray'>,
              TLiteral<'grey'>,
              TLiteral<'blackBright'>,
              TLiteral<'redBright'>,
              TLiteral<'greenBright'>,
              TLiteral<'yellowBright'>,
              TLiteral<'blueBright'>,
              TLiteral<'magentaBright'>,
              TLiteral<'cyanBright'>,
              TLiteral<'whiteBright'>,
            ]
          >;
        }>,
      ]
    >
  >;
  expand: TOptional<TBoolean>;
  extensionsToTreatAsEsm: TOptional<TArray<TString>>;
  fakeTimers: TOptional<
    TIntersect<
      [
        TObject<{
          enableGlobally: TOptional<TBoolean>;
        }>,
        TUnion<
          [
            TObject<{
              advanceTimers: TOptional<TUnion<[TBoolean, TNumber]>>;
              doNotFake: TOptional<
                TArray<
                  TUnion<
                    [
                      TLiteral<'Date'>,
                      TLiteral<'hrtime'>,
                      TLiteral<'nextTick'>,
                      TLiteral<'performance'>,
                      TLiteral<'queueMicrotask'>,
                      TLiteral<'requestAnimationFrame'>,
                      TLiteral<'cancelAnimationFrame'>,
                      TLiteral<'requestIdleCallback'>,
                      TLiteral<'cancelIdleCallback'>,
                      TLiteral<'setImmediate'>,
                      TLiteral<'clearImmediate'>,
                      TLiteral<'setInterval'>,
                      TLiteral<'clearInterval'>,
                      TLiteral<'setTimeout'>,
                      TLiteral<'clearTimeout'>,
                      TLiteral<'Temporal'>,
                    ]
                  >
                >
              >;
              now: TOptional<TInteger>;
              timerLimit: TOptional<TNumber>;
              legacyFakeTimers: TOptional<TLiteral<false>>;
            }>,
            TObject<{
              legacyFakeTimers: TOptional<TLiteral<true>>;
            }>,
          ]
        >,
      ]
    >
  >;
  filter: TOptional<TString>;
  findRelatedTests: TOptional<TBoolean>;
  forceCoverageMatch: TOptional<TArray<TString>>;
  forceExit: TOptional<TBoolean>;
  json: TOptional<TBoolean>;
  globals: TOptional<TRecord<TString, TUnknown>>;
  globalSetup: TOptional<TUnion<[TString, TNull]>>;
  globalTeardown: TOptional<TUnion<[TString, TNull]>>;
  haste: TOptional<
    TObject<{
      computeSha1: TOptional<TBoolean>;
      defaultPlatform: TOptional<TUnion<[TString, TNull]>>;
      forceNodeFilesystemAPI: TOptional<TBoolean>;
      enableSymlinks: TOptional<TBoolean>;
      hasteImplModulePath: TOptional<TString>;
      platforms: TOptional<TArray<TString>>;
      throwOnModuleCollision: TOptional<TBoolean>;
      hasteMapModulePath: TOptional<TString>;
      retainAllFiles: TOptional<TBoolean>;
    }>
  >;
  id: TOptional<TString>;
  injectGlobals: TOptional<TBoolean>;
  reporters: TOptional<
    TArray<TUnion<[TString, TTuple<[TString, TRecord<TString, TUnknown>]>]>>
  >;
  logHeapUsage: TOptional<TBoolean>;
  lastCommit: TOptional<TBoolean>;
  listTests: TOptional<TBoolean>;
  maxConcurrency: TOptional<TInteger>;
  maxWorkers: TOptional<TUnion<[TString, TInteger]>>;
  moduleDirectories: TOptional<TArray<TString>>;
  moduleFileExtensions: TOptional<TArray<TString>>;
  moduleNameMapper: TOptional<
    TRecord<TString, TUnion<[TString, TArray<TString>]>>
  >;
  modulePathIgnorePatterns: TOptional<TArray<TString>>;
  modulePaths: TOptional<TArray<TString>>;
  noStackTrace: TOptional<TBoolean>;
  notify: TOptional<TBoolean>;
  notifyMode: TOptional<TString>;
  onlyChanged: TOptional<TBoolean>;
  onlyFailures: TOptional<TBoolean>;
  openHandlesTimeout: TOptional<TNumber>;
  outputFile: TOptional<TString>;
  passWithNoTests: TOptional<TBoolean>;
  preset: TOptional<TUnion<[TString, TNull]>>;
  prettierPath: TOptional<TUnion<[TString, TNull]>>;
  projects: TOptional<TArray<TUnion<[TString, TRecord<TString, TUnknown>]>>>;
  randomize: TOptional<TBoolean>;
  replname: TOptional<TUnion<[TString, TNull]>>;
  resetMocks: TOptional<TBoolean>;
  resetModules: TOptional<TBoolean>;
  resolver: TOptional<TUnion<[TString, TNull]>>;
  restoreMocks: TOptional<TBoolean>;
  rootDir: TOptional<TString>;
  roots: TOptional<TArray<TString>>;
  runner: TOptional<
    TUnion<[TString, TTuple<[TString, TRecord<TString, TUnknown>]>]>
  >;
  runTestsByPath: TOptional<TBoolean>;
  runtime: TOptional<TString>;
  sandboxInjectedGlobals: TOptional<TArray<TString>>;
  setupFiles: TOptional<TArray<TString>>;
  setupFilesAfterEnv: TOptional<TArray<TString>>;
  showSeed: TOptional<TBoolean>;
  silent: TOptional<TBoolean>;
  skipFilter: TOptional<TBoolean>;
  skipNodeResolution: TOptional<TBoolean>;
  slowTestThreshold: TOptional<TNumber>;
  snapshotResolver: TOptional<TString>;
  snapshotSerializers: TOptional<TArray<TString>>;
  snapshotFormat: TOptional<
    TObject<{
      callToJSON: TOptional<TBoolean>;
      compareKeys: TOptional<TNull>;
      escapeRegex: TOptional<TBoolean>;
      escapeString: TOptional<TBoolean>;
      highlight: TOptional<TBoolean>;
      indent: TOptional<TInteger>;
      maxDepth: TOptional<TInteger>;
      maxWidth: TOptional<TInteger>;
      min: TOptional<TBoolean>;
      printBasicPrototype: TOptional<TBoolean>;
      printFunctionName: TOptional<TBoolean>;
      theme: TOptional<
        TObject<{
          comment: TOptional<TString>;
          content: TOptional<TString>;
          prop: TOptional<TString>;
          tag: TOptional<TString>;
          value: TOptional<TString>;
        }>
      >;
    }>
  >;
  errorOnDeprecated: TOptional<TBoolean>;
  testEnvironment: TOptional<TString>;
  testEnvironmentOptions: TOptional<TRecord<TString, TUnknown>>;
  testFailureExitCode: TOptional<TInteger>;
  testLocationInResults: TOptional<TBoolean>;
  testMatch: TOptional<TUnion<[TString, TArray<TString>]>>;
  testNamePattern: TOptional<TString>;
  testPathIgnorePatterns: TOptional<TArray<TString>>;
  testRegex: TOptional<TUnion<[TString, TArray<TString>]>>;
  testResultsProcessor: TOptional<TString>;
  testRunner: TOptional<TString>;
  testSequencer: TOptional<TString>;
  testTimeout: TOptional<TNumber>;
  transform: TOptional<
    TRecord<TString, TUnion<[TString, TTuple<[TString, TUnknown]>]>>
  >;
  transformIgnorePatterns: TOptional<TArray<TString>>;
  watchPathIgnorePatterns: TOptional<TArray<TString>>;
  unmockedModulePathPatterns: TOptional<TArray<TString>>;
  updateSnapshot: TOptional<TBoolean>;
  useStderr: TOptional<TBoolean>;
  verbose: TOptional<TBoolean>;
  waitForUnhandledRejections: TOptional<TBoolean>;
  watch: TOptional<TBoolean>;
  watchAll: TOptional<TBoolean>;
  watchman: TOptional<TBoolean>;
  watchPlugins: TOptional<
    TArray<TUnion<[TString, TTuple<[TString, TUnknown]>]>>
  >;
  workerIdleMemoryLimit: TOptional<TUnion<[TNumber, TString]>>;
  workerThreads: TOptional<TBoolean>;
  workerGracefulExitTimeout: TOptional<TNumber>;
}>;

export declare type InitialOptions = Static<typeof InitialOptions>;

export declare const SnapshotFormat: TObject<{
  callToJSON: TOptional<TBoolean>;
  compareKeys: TOptional<TNull>;
  escapeRegex: TOptional<TBoolean>;
  escapeString: TOptional<TBoolean>;
  highlight: TOptional<TBoolean>;
  indent: TOptional<TInteger>;
  maxDepth: TOptional<TInteger>;
  maxWidth: TOptional<TInteger>;
  min: TOptional<TBoolean>;
  printBasicPrototype: TOptional<TBoolean>;
  printFunctionName: TOptional<TBoolean>;
  theme: TOptional<
    TObject<{
      comment: TOptional<TString>;
      content: TOptional<TString>;
      prop: TOptional<TString>;
      tag: TOptional<TString>;
      value: TOptional<TString>;
    }>
  >;
}>;

export declare type SnapshotFormat = Static<typeof SnapshotFormat>;
