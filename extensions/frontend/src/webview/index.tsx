// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { render } from 'preact';
import { App } from './App';

const root = document.getElementById('root');
if (root) render(<App />, root);
