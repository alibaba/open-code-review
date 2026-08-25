/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {NapiResolveOptions} from 'unrs-resolver';
import {IModuleMap} from 'jest-haste-map';

export declare type AsyncResolver = (
  path: string,
  options: ResolverOptions,
) => Promise<string>;

declare function cachedShouldLoadAsEsm(
  path: string,
  extensionsToTreatAsEsm: Array<string>,
): boolean;

export declare type FindNodeModuleConfig = {
  basedir: string;
  conditions?: ReadonlyArray<string>;
  extensions?: ReadonlyArray<string>;
  moduleDirectory?: ReadonlyArray<string>;
  paths?: ReadonlyArray<string>;
  resolver?: string | null;
  rootDir?: string;
  throwIfNotFound?: boolean;
};

export declare type JestResolver = ResolverSyncObject | ResolverAsyncObject;

declare interface JSONObject {
  [key: string]: JSONValue;
}

declare type JSONValue =
  | string
  | number
  | boolean
  | JSONObject
  | Array<JSONValue>;

declare type ModuleNameMapperConfig = {
  regex: RegExp;
  moduleName: string | Array<string>;
};

declare class ModuleNotFoundError extends Error {
  code: string;
  hint?: string;
  requireStack?: Array<string>;
  siblingWithSimilarExtensionFound?: boolean;
  moduleName?: string;
  private _originalMessage?;
  constructor(message: string, moduleName?: string);
  buildMessage(rootDir: string): void;
  static duckType(error: ModuleNotFoundError): ModuleNotFoundError;
}

export declare type PackageJSON = JSONObject;

export declare type ResolveModuleConfig = {
  conditions?: ReadonlyArray<string>;
  skipNodeResolution?: boolean;
  paths?: ReadonlyArray<string>;
};

declare class Resolver {
  private readonly _options;
  private readonly _moduleMap;
  private readonly _moduleIDCache;
  private readonly _moduleNameCache;
  private readonly _modulePathCache;
  private readonly _supportsNativePlatform;
  private _canResolveSync;
  constructor(moduleMap: IModuleMap, options: ResolverConfig);
  static ModuleNotFoundError: typeof ModuleNotFoundError;
  static tryCastModuleNotFoundError(error: unknown): ModuleNotFoundError | null;
  static clearDefaultResolverCache(): void;
  static findNodeModule(
    path: string,
    options: FindNodeModuleConfig,
  ): string | null;
  static findNodeModuleAsync(
    path: string,
    options: FindNodeModuleConfig,
  ): Promise<string | null>;
  static unstable_shouldLoadAsEsm: typeof cachedShouldLoadAsEsm;
  resolveModuleFromDirIfExists(
    dirname: string,
    moduleName: string,
    options?: ResolveModuleConfig,
  ): string | null;
  resolveModuleFromDirIfExistsAsync(
    dirname: string,
    moduleName: string,
    options?: ResolveModuleConfig,
  ): Promise<string | null>;
  canResolveSync(): boolean;
  resolveModule(
    from: string,
    moduleName: string,
    options?: ResolveModuleConfig,
  ): string;
  resolveModuleAsync(
    from: string,
    moduleName: string,
    options?: ResolveModuleConfig,
  ): Promise<string>;
  /**
   * _prepareForResolution is shared between the sync and async module resolution
   * methods, to try to keep them as DRY as possible.
   */
  private _prepareForResolution;
  /**
   * _getHasteModulePath attempts to return the path to a haste module.
   */
  private _getHasteModulePath;
  private _throwModNotFoundError;
  private _getMapModuleName;
  private _isAliasModule;
  isCoreModule(moduleName: string): boolean;
  normalizeCoreModuleSpecifier(specifier: string): string;
  getModule(name: string): string | null;
  getModulePath(from: string, moduleName: string): string;
  getPackage(name: string): string | null;
  getMockModule(
    from: string,
    name: string,
    options?: Pick<ResolveModuleConfig, 'conditions'>,
  ): string | null;
  getMockModuleAsync(
    from: string,
    name: string,
    options: Pick<ResolveModuleConfig, 'conditions'>,
  ): Promise<string | null>;
  getModulePaths(from: string): Array<string>;
  getGlobalPaths(moduleName?: string): Array<string>;
  getModuleID(
    virtualMocks: Map<string, boolean>,
    from: string,
    moduleName: string | undefined,
    options: ResolveModuleConfig,
  ): string;
  getModuleIDAsync(
    virtualMocks: Map<string, boolean>,
    from: string,
    moduleName: string | undefined,
    options: ResolveModuleConfig,
  ): Promise<string>;
  private _getModuleType;
  private _getAbsolutePath;
  private _getAbsolutePathAsync;
  private _getMockPath;
  private _getMockPathAsync;
  private _getVirtualMockPath;
  private _getVirtualMockPathAsync;
  private _isModuleResolved;
  private _isModuleResolvedAsync;
  resolveStubModuleName(
    from: string,
    moduleName: string,
    options?: Pick<ResolveModuleConfig, 'conditions'>,
  ): string | null;
  resolveStubModuleNameAsync(
    from: string,
    moduleName: string,
    options?: Pick<ResolveModuleConfig, 'conditions'>,
  ): Promise<string | null>;
}
export default Resolver;

declare type ResolverAsyncObject = {
  sync?: SyncResolver;
  async: AsyncResolver;
};

declare type ResolverConfig = {
  defaultPlatform?: string | null;
  extensions: Array<string>;
  hasCoreModules: boolean;
  moduleDirectories: Array<string>;
  moduleNameMapper?: Array<ModuleNameMapperConfig> | null;
  modulePaths?: Array<string>;
  platforms?: Array<string>;
  resolver?: string | null;
  rootDir: string;
};

export declare interface ResolverOptions extends Omit<
  NapiResolveOptions,
  'extensions'
> {
  /** Directory to begin resolving from. */
  basedir: string;
  /** List of export conditions. */
  conditions?: ReadonlyArray<string>;
  /** Instance of default resolver. */
  defaultResolver: SyncResolver;
  /** Instance of default async resolver. */
  defaultAsyncResolver: AsyncResolver;
  /** List of file extensions to be considered when resolving. */
  extensions?: ReadonlyArray<string>;
  /**
   * List of directory names to be looked up for modules recursively.
   *
   * @defaultValue
   * The default is `['node_modules']`.
   */
  moduleDirectory?: ReadonlyArray<string>;
  /**
   * List of `require.paths` to use if nothing is found in `node_modules`.
   *
   * @defaultValue
   * The default is `undefined`.
   */
  paths?: ReadonlyArray<string>;
  /** Current root directory. */
  rootDir?: string;
}

declare type ResolverSyncObject = {
  sync: SyncResolver;
  async?: AsyncResolver;
};

/**
 * Finds the runner to use:
 *
 * 1. looks for jest-runner-<name> relative to project.
 * 1. looks for jest-runner-<name> relative to Jest.
 * 1. looks for <name> relative to project.
 * 1. looks for <name> relative to Jest.
 */
export declare const resolveRunner: (
  resolver: string | undefined | null,
  {
    filePath,
    rootDir,
    requireResolveFunction,
  }: {
    filePath: string;
    rootDir: string;
    requireResolveFunction: (moduleName: string) => string;
  },
) => string;

export declare const resolveSequencer: (
  resolver: string | undefined | null,
  {
    filePath,
    rootDir,
    requireResolveFunction,
  }: {
    filePath: string;
    rootDir: string;
    requireResolveFunction: (moduleName: string) => string;
  },
) => string;

/**
 * Finds the test environment to use:
 *
 * 1. looks for jest-environment-<name> relative to project.
 * 1. looks for jest-environment-<name> relative to Jest.
 * 1. looks for <name> relative to project.
 * 1. looks for <name> relative to Jest.
 */
export declare const resolveTestEnvironment: ({
  rootDir,
  testEnvironment: filePath,
  requireResolveFunction,
}: {
  rootDir: string;
  testEnvironment: string;
  requireResolveFunction: (moduleName: string) => string;
}) => string;

/**
 * Finds the watch plugins to use:
 *
 * 1. looks for jest-watch-<name> relative to project.
 * 1. looks for jest-watch-<name> relative to Jest.
 * 1. looks for <name> relative to project.
 * 1. looks for <name> relative to Jest.
 */
export declare const resolveWatchPlugin: (
  resolver: string | undefined | null,
  {
    filePath,
    rootDir,
    requireResolveFunction,
  }: {
    filePath: string;
    rootDir: string;
    requireResolveFunction: (moduleName: string) => string;
  },
) => string;

export declare type SyncResolver = (
  path: string,
  options: ResolverOptions,
) => string;
