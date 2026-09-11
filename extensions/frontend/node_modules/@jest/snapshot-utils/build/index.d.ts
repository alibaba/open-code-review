/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {Config} from '@jest/types';

export declare const ensureDirectoryExists: (filePath: string) => void;

export declare const escapeBacktickString: (str: string) => string;

export declare const getSnapshotData: (
  snapshotPath: string,
  update: Config.SnapshotUpdateState,
) => {
  data: SnapshotData;
  dirty: boolean;
};

export declare const keyToTestName: (key: string) => string;

export declare const normalizeNewlines: (string: string) => string;

export declare const saveSnapshotFile: (
  snapshotData: SnapshotData,
  snapshotPath: string,
) => void;

export declare const SNAPSHOT_GUIDE_LINK =
  'https://jestjs.io/docs/snapshot-testing';

export declare const SNAPSHOT_VERSION = '1';

export declare const SNAPSHOT_VERSION_WARNING: string;

export declare type SnapshotData = Record<string, string>;

export declare const testNameToKey: (testName: string, count: number) => string;
