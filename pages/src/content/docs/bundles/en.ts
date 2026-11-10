// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { DocLanguageBundle } from '..';
import quickstart from '../en/quickstart.md';
import installation from '../en/installation.md';
import configuration from '../en/configuration.md';
import chatgptPreview from '../en/chatgpt-preview.md';
import cliReference from '../en/cli-reference.md';
import reviewRules from '../en/review-rules.md';
import architecture from '../en/architecture.md';
import tools from '../en/tools.md';
import mcp from '../en/mcp.md';
import viewer from '../en/viewer.md';
import telemetry from '../en/telemetry.md';
import agentSkill from '../en/integrations/agent-skill.md';
import claudeCode from '../en/integrations/claude-code.md';
import cicd from '../en/integrations/ci.md';
import delegate from '../en/integrations/delegate.md';
import acp from '../en/integrations/acp.md';
import contributing from '../en/contributing.md';
import faq from '../en/faq.md';

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
