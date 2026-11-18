// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* Japanese docs content bundle — loaded on demand via loadLanguageContent in index.ts. */

import type { LocalizedDocs } from './types';

import jaQuickstart from './ja/quickstart.md';
import jaInstallation from './ja/installation.md';
import jaConfiguration from './ja/configuration.md';
import jaChatGPTPreview from './ja/chatgpt-preview.md';
import jaCliReference from './ja/cli-reference.md';
import jaReviewRules from './ja/review-rules.md';
import jaArchitecture from './ja/architecture.md';
import jaTools from './ja/tools.md';
import jaMcp from './ja/mcp.md';
import jaViewer from './ja/viewer.md';
import jaTelemetry from './ja/telemetry.md';
import jaAgentSkill from './ja/integrations/agent-skill.md';
import jaClaudeCode from './ja/integrations/claude-code.md';
import jaCicd from './ja/integrations/ci.md';
import jaDelegate from './ja/integrations/delegate.md';
import jaAcp from './ja/integrations/acp.md';
import jaCopilotCli from './ja/integrations/copilot-cli.md';
import jaContributing from './ja/contributing.md';
import jaFaq from './ja/faq.md';

const jaDocs: LocalizedDocs = {
  'quickstart': jaQuickstart,
  'installation': jaInstallation,
  'configuration': jaConfiguration,
  'chatgpt-preview': jaChatGPTPreview,
  'cli-reference': jaCliReference,
  'review-rules': jaReviewRules,
  'architecture': jaArchitecture,
  'tools': jaTools,
  'mcp': jaMcp,
  'viewer': jaViewer,
  'telemetry': jaTelemetry,
  'agent-skill': jaAgentSkill,
  'claude-code': jaClaudeCode,
  'cicd': jaCicd,
  'delegate': jaDelegate,
  'acp': jaAcp,
  'copilot-cli': jaCopilotCli,
  'contributing': jaContributing,
  'faq': jaFaq,
};

export default jaDocs;
