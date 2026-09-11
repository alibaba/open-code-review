/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {Context} from 'node:vm';
import {
  EnvironmentContext,
  JestEnvironment,
  JestEnvironmentConfig,
} from '@jest/environment';
import {LegacyFakeTimers, ModernFakeTimers} from '@jest/fake-timers';
import {Global as Global_2} from '@jest/types';
import {ModuleMocker} from 'jest-mock';

declare class NodeEnvironment implements JestEnvironment<Timer> {
  context: Context | null;
  fakeTimers: LegacyFakeTimers<Timer> | null;
  fakeTimersModern: ModernFakeTimers | null;
  global: Global_2.Global;
  moduleMocker: ModuleMocker | null;
  customExportConditions: Array<string>;
  private readonly _configuredExportConditions?;
  private _globalProxy;
  constructor(config: JestEnvironmentConfig, _context: EnvironmentContext);
  setup(): Promise<void>;
  teardown(): Promise<void>;
  exportConditions(): Array<string>;
  getVmContext(): Context | null;
}
export default NodeEnvironment;

export declare const TestEnvironment: typeof NodeEnvironment;

declare type Timer = {
  id: number;
  ref: () => Timer;
  unref: () => Timer;
};
