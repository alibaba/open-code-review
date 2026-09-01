/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {Global as Global_2} from '@jest/types';

export declare function bind<EachCallback extends Global_2.TestCallback>(
  cb: GlobalCallback,
  supportsDone?: boolean,
  needsEachError?: boolean,
): Global_2.EachTestFn<any>;

declare const each: {
  (
    table: Global_2.EachTable,
    ...data: Global_2.TemplateData
  ): ReturnType<typeof install>;
  withGlobal(g: Global_2): (
    table: Global_2.EachTable,
    ...data: Global_2.TemplateData
  ) => {
    describe: {
      (
        title: string,
        suite: Global_2.EachTestFn<Global_2.BlockFn>,
        timeout?: number,
      ): any;
      skip: any;
      only: any;
    };
    fdescribe: any;
    fit: any;
    it: {
      (
        title: string,
        test: Global_2.EachTestFn<Global_2.TestFn>,
        timeout?: number,
      ): any;
      skip: any;
      only: any;
      concurrent: {
        (
          title: string,
          test: Global_2.EachTestFn<Global_2.TestFn>,
          timeout?: number,
        ): any;
        only: any;
        skip: any;
      };
    };
    test: {
      (
        title: string,
        test: Global_2.EachTestFn<Global_2.TestFn>,
        timeout?: number,
      ): any;
      skip: any;
      only: any;
      concurrent: {
        (
          title: string,
          test: Global_2.EachTestFn<Global_2.TestFn>,
          timeout?: number,
        ): any;
        only: any;
        skip: any;
      };
    };
    xdescribe: any;
    xit: any;
    xtest: any;
  };
};
export default each;

declare type GlobalCallback = (
  testName: string,
  fn: Global_2.ConcurrentTestFn,
  timeout?: number,
  eachError?: Error,
) => void;

declare const install: (
  g: Global_2,
  table: Global_2.EachTable,
  ...data: Global_2.TemplateData
) => {
  describe: {
    (
      title: string,
      suite: Global_2.EachTestFn<Global_2.BlockFn>,
      timeout?: number,
    ): any;
    skip: any;
    only: any;
  };
  fdescribe: any;
  fit: any;
  it: {
    (
      title: string,
      test: Global_2.EachTestFn<Global_2.TestFn>,
      timeout?: number,
    ): any;
    skip: any;
    only: any;
    concurrent: {
      (
        title: string,
        test: Global_2.EachTestFn<Global_2.TestFn>,
        timeout?: number,
      ): any;
      only: any;
      skip: any;
    };
  };
  test: {
    (
      title: string,
      test: Global_2.EachTestFn<Global_2.TestFn>,
      timeout?: number,
    ): any;
    skip: any;
    only: any;
    concurrent: {
      (
        title: string,
        test: Global_2.EachTestFn<Global_2.TestFn>,
        timeout?: number,
      ): any;
      only: any;
      skip: any;
    };
  };
  xdescribe: any;
  xit: any;
  xtest: any;
};
