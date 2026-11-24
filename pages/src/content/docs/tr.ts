// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { CompleteLocalizedDocs } from './types';

import trQuickstart from './tr/quickstart.md';
import trInstallation from './tr/installation.md';
import trConfiguration from './tr/configuration.md';
import trChatGPTPreview from './tr/chatgpt-preview.md';
import trCliReference from './tr/cli-reference.md';
import trReviewRules from './tr/review-rules.md';
import trReviewingRequirements from './tr/reviewing-requirements.md';
import trArchitecture from './tr/architecture.md';
import trTools from './tr/tools.md';
import trMcp from './tr/mcp.md';
import trViewer from './tr/viewer.md';
import trTelemetry from './tr/telemetry.md';
import trAgentSkill from './tr/integrations/agent-skill.md';
import trClaudeCode from './tr/integrations/claude-code.md';
import trCicd from './tr/integrations/ci.md';
import trDelegate from './tr/integrations/delegate.md';
import trAcp from './tr/integrations/acp.md';
import trCopilotCli from './tr/integrations/copilot-cli.md';
import trContributing from './tr/contributing.md';
import trFaq from './tr/faq.md';

const trDocs: CompleteLocalizedDocs = {
  'quickstart': trQuickstart,
  'installation': trInstallation,
  'configuration': trConfiguration,
  'chatgpt-preview': trChatGPTPreview,
  'cli-reference': trCliReference,
  'review-rules': trReviewRules,
  'reviewing-requirements': trReviewingRequirements,
  'architecture': trArchitecture,
  'tools': trTools,
  'mcp': trMcp,
  'viewer': trViewer,
  'telemetry': trTelemetry,
  'agent-skill': trAgentSkill,
  'claude-code': trClaudeCode,
  'cicd': trCicd,
  'delegate': trDelegate,
  'acp': trAcp,
  'copilot-cli': trCopilotCli,
  'contributing': trContributing,
  'faq': trFaq,
};

export default trDocs;
