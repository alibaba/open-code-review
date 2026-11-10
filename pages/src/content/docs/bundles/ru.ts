// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { DocLanguageBundle } from '..';
import quickstart from '../ru/quickstart.md';
import installation from '../ru/installation.md';
import configuration from '../ru/configuration.md';
import chatgptPreview from '../ru/chatgpt-preview.md';
import cliReference from '../ru/cli-reference.md';
import reviewRules from '../ru/review-rules.md';
import architecture from '../ru/architecture.md';
import tools from '../ru/tools.md';
import mcp from '../ru/mcp.md';
import viewer from '../ru/viewer.md';
import telemetry from '../ru/telemetry.md';
import agentSkill from '../ru/integrations/agent-skill.md';
import claudeCode from '../ru/integrations/claude-code.md';
import cicd from '../ru/integrations/ci.md';
import delegate from '../ru/integrations/delegate.md';
import acp from '../ru/integrations/acp.md';
import contributing from '../ru/contributing.md';
import faq from '../ru/faq.md';

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
