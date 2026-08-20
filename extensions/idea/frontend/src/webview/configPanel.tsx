// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { render } from 'preact';
import { ConfigPanelApp } from './ConfigPanelApp';

const root = document.getElementById('root');
if (root) render(<ConfigPanelApp />, root);
