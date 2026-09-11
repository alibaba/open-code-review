/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

import {PluginObj, babelCore} from '@babel/core';
import {Identifier} from '@babel/types';

declare function jestHoist(babel: babelCore): PluginObj<{
  declareJestObjGetterIdentifier: () => Identifier;
  jestObjGetterIdentifier?: Identifier;
}>;
export default jestHoist;
