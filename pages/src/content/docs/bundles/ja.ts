// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { DocLanguageBundle } from '..';
import quickstart from '../ja/quickstart.md';
import installation from '../ja/installation.md';
import configuration from '../ja/configuration.md';
import chatgptPreview from '../ja/chatgpt-preview.md';
import cliReference from '../ja/cli-reference.md';
import reviewRules from '../ja/review-rules.md';
import architecture from '../ja/architecture.md';
import tools from '../ja/tools.md';
import mcp from '../ja/mcp.md';
import viewer from '../ja/viewer.md';
import telemetry from '../ja/telemetry.md';
import agentSkill from '../ja/integrations/agent-skill.md';
import claudeCode from '../ja/integrations/claude-code.md';
import cicd from '../ja/integrations/ci.md';
import delegate from '../ja/integrations/delegate.md';
import acp from '../ja/integrations/acp.md';
import contributing from '../ja/contributing.md';
import faq from '../ja/faq.md';

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
