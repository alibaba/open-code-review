// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* Russian docs content bundle — loaded on demand via loadLanguageContent in index.ts.
 * Russian is an incremental locale: entries missing here fall back to English. */

import type { LocalizedDocs } from './types';

import ruQuickstart from './ru/quickstart.md';
import ruInstallation from './ru/installation.md';
import ruConfiguration from './ru/configuration.md';
import ruChatGPTPreview from './ru/chatgpt-preview.md';
import ruCliReference from './ru/cli-reference.md';
import ruReviewRules from './ru/review-rules.md';
import ruArchitecture from './ru/architecture.md';
import ruTools from './ru/tools.md';
import ruMcp from './ru/mcp.md';
import ruViewer from './ru/viewer.md';
import ruTelemetry from './ru/telemetry.md';
import ruAgentSkill from './ru/integrations/agent-skill.md';
import ruClaudeCode from './ru/integrations/claude-code.md';
import ruCicd from './ru/integrations/ci.md';
import ruDelegate from './ru/integrations/delegate.md';
import ruAcp from './ru/integrations/acp.md';
import ruCopilotCli from './ru/integrations/copilot-cli.md';
import ruContributing from './ru/contributing.md';
import ruFaq from './ru/faq.md';

const ruDocs: LocalizedDocs = {
  'quickstart': ruQuickstart,
  'installation': ruInstallation,
  'configuration': ruConfiguration,
  'chatgpt-preview': ruChatGPTPreview,
  'cli-reference': ruCliReference,
  'review-rules': ruReviewRules,
  'architecture': ruArchitecture,
  'tools': ruTools,
  'mcp': ruMcp,
  'viewer': ruViewer,
  'telemetry': ruTelemetry,
  'agent-skill': ruAgentSkill,
  'claude-code': ruClaudeCode,
  'cicd': ruCicd,
  'delegate': ruDelegate,
  'acp': ruAcp,
  'copilot-cli': ruCopilotCli,
  'contributing': ruContributing,
  'faq': ruFaq,
};

export default ruDocs;
