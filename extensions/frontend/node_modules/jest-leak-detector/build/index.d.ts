/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

declare class LeakDetector {
  private _isReferenceBeingHeld;
  private _shouldGenerateV8HeapSnapshot;
  private readonly _finalizationRegistry?;
  constructor(value: unknown, opt?: LeakDetectorOptions);
  isLeaking(): Promise<boolean>;
  private _runGarbageCollector;
}
export default LeakDetector;

declare interface LeakDetectorOptions {
  shouldGenerateV8HeapSnapshot: boolean;
}
