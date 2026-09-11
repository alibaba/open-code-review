/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {
  SearchSource,
  createTestScheduler,
  getVersion,
  runCLI,
} from '@jest/core';
import {Config as Config_2} from '@jest/types';
import {buildArgv, run} from 'jest-cli';
import {defineConfig, mergeConfig} from 'jest-config';

export {buildArgv};

export declare type Config = Config_2.InitialOptions;

export {createTestScheduler};

export {defineConfig};

export {getVersion};

export declare type GlobalConfig = Config_2.GlobalConfig;

export {mergeConfig};

export declare type ProjectConfig = Config_2.ProjectConfig;

export {run};

export {runCLI};

export {SearchSource};
