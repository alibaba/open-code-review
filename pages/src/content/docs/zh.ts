// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* Chinese docs content bundle — loaded on demand via loadLanguageContent in index.ts. */

import type { LocalizedDocs } from './types';

import zhQuickstart from './zh/quickstart.md';
import zhInstallation from './zh/installation.md';
import zhConfiguration from './zh/configuration.md';
import zhChatGPTPreview from './zh/chatgpt-preview.md';
import zhCliReference from './zh/cli-reference.md';
import zhReviewRules from './zh/review-rules.md';
import zhArchitecture from './zh/architecture.md';
import zhTools from './zh/tools.md';
import zhMcp from './zh/mcp.md';
import zhViewer from './zh/viewer.md';
import zhTelemetry from './zh/telemetry.md';
import zhAgentSkill from './zh/integrations/agent-skill.md';
import zhClaudeCode from './zh/integrations/claude-code.md';
import zhCicd from './zh/integrations/ci.md';
import zhDelegate from './zh/integrations/delegate.md';
import zhAcp from './zh/integrations/acp.md';
import zhCopilotCli from './zh/integrations/copilot-cli.md';
import zhContributing from './zh/contributing.md';
import zhFaq from './zh/faq.md';

const zhDocs: LocalizedDocs = {
  'quickstart': zhQuickstart,
  'installation': zhInstallation,
  'configuration': zhConfiguration,
  'chatgpt-preview': zhChatGPTPreview,
  'cli-reference': zhCliReference,
  'review-rules': zhReviewRules,
  'architecture': zhArchitecture,
  'tools': zhTools,
  'mcp': zhMcp,
  'viewer': zhViewer,
  'telemetry': zhTelemetry,
  'agent-skill': zhAgentSkill,
  'claude-code': zhClaudeCode,
  'cicd': zhCicd,
  'delegate': zhDelegate,
  'acp': zhAcp,
  'copilot-cli': zhCopilotCli,
  'contributing': zhContributing,
  'faq': zhFaq,
};

export default zhDocs;
