// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* English docs content bundle — loaded on demand via loadLanguageContent in index.ts. */

import type { LocalizedDocs } from './types';

import enQuickstart from './en/quickstart.md';
import enInstallation from './en/installation.md';
import enConfiguration from './en/configuration.md';
import enChatGPTPreview from './en/chatgpt-preview.md';
import enCliReference from './en/cli-reference.md';
import enReviewRules from './en/review-rules.md';
import enArchitecture from './en/architecture.md';
import enTools from './en/tools.md';
import enMcp from './en/mcp.md';
import enViewer from './en/viewer.md';
import enTelemetry from './en/telemetry.md';
import enAgentSkill from './en/integrations/agent-skill.md';
import enClaudeCode from './en/integrations/claude-code.md';
import enCicd from './en/integrations/ci.md';
import enDelegate from './en/integrations/delegate.md';
import enAcp from './en/integrations/acp.md';
import enCopilotCli from './en/integrations/copilot-cli.md';
import enContributing from './en/contributing.md';
import enFaq from './en/faq.md';

const enDocs: LocalizedDocs = {
  'quickstart': enQuickstart,
  'installation': enInstallation,
  'configuration': enConfiguration,
  'chatgpt-preview': enChatGPTPreview,
  'cli-reference': enCliReference,
  'review-rules': enReviewRules,
  'architecture': enArchitecture,
  'tools': enTools,
  'mcp': enMcp,
  'viewer': enViewer,
  'telemetry': enTelemetry,
  'agent-skill': enAgentSkill,
  'claude-code': enClaudeCode,
  'cicd': enCicd,
  'delegate': enDelegate,
  'acp': enAcp,
  'copilot-cli': enCopilotCli,
  'contributing': enContributing,
  'faq': enFaq,
};

export default enDocs;
