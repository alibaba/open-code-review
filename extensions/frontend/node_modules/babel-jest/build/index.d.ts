/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {TransformOptions} from '@babel/core';
import {SyncTransformer, TransformerCreator} from '@jest/transform';

export declare const createTransformer: TransformerCreator<
  SyncTransformer<TransformerConfig>,
  TransformerConfig
>;

export declare interface TransformerConfig extends TransformOptions {
  excludeJestPreset?: boolean;
}

declare const transformerFactory: {
  createTransformer: TransformerCreator<
    SyncTransformer<TransformerConfig>,
    TransformerConfig
  >;
};
export default transformerFactory;
