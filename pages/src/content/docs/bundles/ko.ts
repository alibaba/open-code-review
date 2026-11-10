// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { DocLanguageBundle } from '..';
import quickstart from '../ko/quickstart.md';
import installation from '../ko/installation.md';
import configuration from '../ko/configuration.md';
import chatgptPreview from '../ko/chatgpt-preview.md';
import cliReference from '../ko/cli-reference.md';
import reviewRules from '../ko/review-rules.md';
import architecture from '../ko/architecture.md';
import tools from '../ko/tools.md';
import mcp from '../ko/mcp.md';
import viewer from '../ko/viewer.md';
import telemetry from '../ko/telemetry.md';
import agentSkill from '../ko/integrations/agent-skill.md';
import claudeCode from '../ko/integrations/claude-code.md';
import cicd from '../ko/integrations/ci.md';
import delegate from '../ko/integrations/delegate.md';
import acp from '../ko/integrations/acp.md';
import contributing from '../ko/contributing.md';
import faq from '../ko/faq.md';

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
