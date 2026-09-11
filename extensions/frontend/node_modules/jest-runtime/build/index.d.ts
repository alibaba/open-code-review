/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {JestEnvironment} from '@jest/environment';
import {expect} from '@jest/globals';
import {SourceMapRegistry} from '@jest/source-map';
import {TestContext, V8CoverageResult} from '@jest/test-result';
import {
  CallerTransformOptions,
  ScriptTransformer,
  ShouldInstrumentOptions,
  shouldInstrument,
} from '@jest/transform';
import {Config, Global as Global_2} from '@jest/types';
import {IHasteMap, IModuleMap} from 'jest-haste-map';
import Resolver from 'jest-resolve';

declare interface EnvironmentGlobals extends Global_2.TestFrameworkGlobals {
  expect: typeof expect;
}

declare type HasteMapOptions = {
  console?: Console;
  maxWorkers: number;
  resetCache: boolean;
  watch?: boolean;
  watchman: boolean;
  workerThreads?: boolean;
};

declare class Runtime {
  private readonly fileCache;
  private readonly _config;
  private readonly _coverageOptions;
  private readonly _environment;
  private readonly mockState;
  private readonly registries;
  private readonly testMainModule;
  private readonly requireBuilder;
  private readonly executor;
  private readonly esmLoader;
  private readonly cjsLoader;
  private readonly _moduleMocker;
  private readonly cjsExportsCache;
  private readonly _testPath;
  private readonly _resolution;
  private readonly transformCache;
  private readonly v8Coverage;
  private readonly coreModule;
  private readonly jestGlobals;
  private readonly testState;
  private readonly loggedReferenceErrors;
  constructor(
    config: Config.ProjectConfig,
    environment: JestEnvironment,
    resolver: Resolver,
    transformer: ScriptTransformer,
    cacheFS: Map<string, string>,
    coverageOptions: ShouldInstrumentOptions,
    testPath: string,
    globalConfig: Config.GlobalConfig,
  );
  static shouldInstrument: typeof shouldInstrument;
  static createContext(
    config: Config.ProjectConfig,
    options: {
      console?: Console;
      maxWorkers: number;
      watch?: boolean;
      watchman: boolean;
    },
  ): Promise<TestContext>;
  static createHasteMap(
    config: Config.ProjectConfig,
    options?: HasteMapOptions,
  ): Promise<IHasteMap>;
  static createResolver(
    config: Config.ProjectConfig,
    moduleMap: IModuleMap,
  ): Resolver;
  unstable_shouldLoadAsEsm(modulePath: string): boolean;
  unstable_importModule(
    from: string,
    moduleName?: string,
  ): Promise<unknown | void>;
  requireModule<T = unknown>(
    from: string,
    moduleName?: string,
    options?: TransformOptions,
    isRequireActual?: boolean,
  ): T;
  requireInternalModule<T = unknown>(from: string, to?: string): T;
  requireActual<T = unknown>(from: string, moduleName: string): T;
  requireMock<T = unknown>(from: string, moduleName: string): T;
  private _requireMockWithId;
  private _getFullTransformationOptions;
  requireModuleOrMock<T = unknown>(from: string, moduleName: string): T;
  isolateModules(fn: () => void): void;
  isolateModulesAsync(fn: () => Promise<void>): Promise<void>;
  resetModules(): void;
  collectV8Coverage(): Promise<void>;
  stopCollectingV8Coverage(): Promise<void>;
  getAllCoverageInfoCopy(): JestEnvironment['global']['__coverage__'];
  getAllV8CoverageInfoCopy(): V8CoverageResult;
  getSourceMaps(): SourceMapRegistry;
  setMock(
    from: string,
    moduleName: string,
    mockFactory: () => unknown,
    options?: {
      virtual?: boolean;
    },
  ): void;
  private setModuleMock;
  restoreAllMocks(): void;
  resetAllMocks(): void;
  clearAllMocks(): void;
  enterTestCode(): void;
  leaveTestCode(): void;
  teardown(): void;
  private _generateMock;
  private _logFormattedReferenceError;
  setGlobalsForRuntime(globals: EnvironmentGlobals): void;
}
export default Runtime;

declare interface TransformOptions extends Required<CallerTransformOptions> {
  isInternalModule: boolean;
}
