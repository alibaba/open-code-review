// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* Korean docs content bundle — loaded on demand via loadLanguageContent in index.ts. */

import type { LocalizedDocs } from './types';

import koQuickstart from './ko/quickstart.md';
import koInstallation from './ko/installation.md';
import koConfiguration from './ko/configuration.md';
import koChatGPTPreview from './ko/chatgpt-preview.md';
import koCliReference from './ko/cli-reference.md';
import koReviewRules from './ko/review-rules.md';
import koArchitecture from './ko/architecture.md';
import koTools from './ko/tools.md';
import koMcp from './ko/mcp.md';
import koViewer from './ko/viewer.md';
import koTelemetry from './ko/telemetry.md';
import koAgentSkill from './ko/integrations/agent-skill.md';
import koClaudeCode from './ko/integrations/claude-code.md';
import koCicd from './ko/integrations/ci.md';
import koDelegate from './ko/integrations/delegate.md';
import koAcp from './ko/integrations/acp.md';
import koCopilotCli from './ko/integrations/copilot-cli.md';
import koContributing from './ko/contributing.md';
import koFaq from './ko/faq.md';

const koDocs: LocalizedDocs = {
  'quickstart': koQuickstart,
  'installation': koInstallation,
  'configuration': koConfiguration,
  'chatgpt-preview': koChatGPTPreview,
  'cli-reference': koCliReference,
  'review-rules': koReviewRules,
  'architecture': koArchitecture,
  'tools': koTools,
  'mcp': koMcp,
  'viewer': koViewer,
  'telemetry': koTelemetry,
  'agent-skill': koAgentSkill,
  'claude-code': koClaudeCode,
  'cicd': koCicd,
  'delegate': koDelegate,
  'acp': koAcp,
  'copilot-cli': koCopilotCli,
  'contributing': koContributing,
  'faq': koFaq,
};

export default koDocs;
