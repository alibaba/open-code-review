/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {Circus, Global as Global_2} from '@jest/types';

export declare const addEventHandler: (handler: Circus.EventHandler) => void;

export declare const afterAll: THook;

export declare const afterEach: THook;

export declare const beforeAll: THook;

export declare const beforeEach: THook;

declare const _default: {
  afterAll: THook;
  afterEach: THook;
  beforeAll: THook;
  beforeEach: THook;
  describe: {
    (blockName: Circus.BlockNameLike, blockFn: Circus.BlockFn): void;
    each: Global_2.EachTestFn<any>;
    only: {
      (blockName: Circus.BlockNameLike, blockFn: Circus.BlockFn): void;
      each: Global_2.EachTestFn<any>;
    };
    skip: {
      (blockName: Circus.BlockNameLike, blockFn: Circus.BlockFn): void;
      each: Global_2.EachTestFn<any>;
    };
  };
  it: Global_2.It;
  test: Global_2.It;
};
export default _default;

export declare const describe: {
  (blockName: Circus.BlockNameLike, blockFn: Circus.BlockFn): void;
  each: Global_2.EachTestFn<any>;
  only: {
    (blockName: Circus.BlockNameLike, blockFn: Circus.BlockFn): void;
    each: Global_2.EachTestFn<any>;
  };
  skip: {
    (blockName: Circus.BlockNameLike, blockFn: Circus.BlockFn): void;
    each: Global_2.EachTestFn<any>;
  };
};

declare type Event_2 = Circus.Event;
export {Event_2 as Event};

export declare const getState: () => Circus.State;

export declare const it: Global_2.It;

export declare const removeEventHandler: (handler: Circus.EventHandler) => void;

export declare const resetState: () => void;

export declare const run: () => Promise<Circus.RunResult>;

export declare const setState: (state: Circus.State) => Circus.State;

export declare type State = Circus.State;

export declare const test: Global_2.It;

declare type THook = (fn: Circus.HookFn, timeout?: number) => void;
