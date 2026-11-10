// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { DocLanguageBundle } from '..';
import quickstart from '../zh/quickstart.md';
import installation from '../zh/installation.md';
import configuration from '../zh/configuration.md';
import chatgptPreview from '../zh/chatgpt-preview.md';
import cliReference from '../zh/cli-reference.md';
import reviewRules from '../zh/review-rules.md';
import architecture from '../zh/architecture.md';
import tools from '../zh/tools.md';
import mcp from '../zh/mcp.md';
import viewer from '../zh/viewer.md';
import telemetry from '../zh/telemetry.md';
import agentSkill from '../zh/integrations/agent-skill.md';
import claudeCode from '../zh/integrations/claude-code.md';
import cicd from '../zh/integrations/ci.md';
import delegate from '../zh/integrations/delegate.md';
import acp from '../zh/integrations/acp.md';
import contributing from '../zh/contributing.md';
import faq from '../zh/faq.md';

const docs: DocLanguageBundle = {
  quickstart,
  installation,
  configuration,
  'chatgpt-preview': chatgptPreview,
  'cli-reference': cliReference,
  'review-rules': reviewRules,
  architecture,
  tools,
  mcp,
  viewer,
  telemetry,
  'agent-skill': agentSkill,
  'claude-code': claudeCode,
  cicd,
  delegate,
  acp,
  contributing,
  faq,
};

export default docs;
