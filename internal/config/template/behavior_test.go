// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package template

import (
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/toolsconfig"
)

func TestBehaviorTemplateIsolation(test *testing.T) {
	behavior, err := LoadBehaviorDefault()
	if err != nil {
		test.Fatal(err)
	}
	behavior.ApplyLanguage("")
	if !strings.Contains(behavior.MainTask.Messages[0].Content, "Write prose in English") || !strings.Contains(behavior.MemoryCompressionTask.Messages[0].Content, "observation IDs") {
		test.Fatal("missing language or evidence preservation")
	}
	catalog, err := toolsconfig.LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	if definitions, err := toolsconfig.Resolve(catalog, behavior.MainTask.Tools); err != nil || len(definitions) != 6 {
		test.Fatalf("definitions: %v, %v", definitions, err)
	}
	review, err := LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	scan, err := LoadScanDefault()
	if err != nil {
		test.Fatal(err)
	}
	for _, conversation := range []LlmConversation{review.MainTask, scan.MainTask} {
		for _, name := range conversation.Tools {
			if name == "context_read" || name == "submit_analysis" {
				test.Fatal("behavior tools leaked into legacy template")
			}
		}
	}
}

func TestBehaviorMainTaskMessageStructure(test *testing.T) {
	behavior, err := LoadBehaviorDefault()
	if err != nil {
		test.Fatal(err)
	}
	messages := behavior.MainTask.Messages
	if len(messages) != 2 || messages[0].Role != "system" || messages[1].Role != "user" {
		test.Fatal("behavior main task must provide a system/user prefix without message merging")
	}
	for _, heading := range []string{"# Role", "# Evidence and trust", "# Investigation protocol", "# Conditional investigation rules"} {
		if !strings.Contains(messages[0].Content, heading) {
			test.Errorf("system message is missing %q", heading)
		}
	}
	for _, placeholder := range []string{"{{comparison_manifest}}", "{{background_context}}", "{{change_inventory}}", "{{context_index}}"} {
		if !strings.Contains(messages[1].Content, placeholder) || strings.Contains(messages[0].Content, placeholder) {
			test.Errorf("task input %q must remain in the user message", placeholder)
		}
	}
}

func TestBehaviorTemplateValidation(test *testing.T) {
	for _, mutate := range []func(*BehaviorTemplate){
		func(tpl *BehaviorTemplate) { tpl.Version = "" },
		func(tpl *BehaviorTemplate) { tpl.MaxTokens = 0 },
		func(tpl *BehaviorTemplate) { tpl.MaxCompletionTokens = 0 },
		func(tpl *BehaviorTemplate) { tpl.MaxToolRequestTimes = 0 },
		func(tpl *BehaviorTemplate) { tpl.MainTask.Tools = append(tpl.MainTask.Tools, "task_done") },
		func(tpl *BehaviorTemplate) { tpl.FinalRound.Tools = nil },
		func(tpl *BehaviorTemplate) { tpl.MemoryCompressionTask.Tools = []string{"file_read"} },
		func(tpl *BehaviorTemplate) { tpl.MainTask.Messages = nil },
		func(tpl *BehaviorTemplate) { tpl.MainTask.ToolReferences = []string{"file_read"} },
	} {
		tpl, err := LoadBehaviorDefault()
		if err != nil {
			test.Fatal(err)
		}
		mutate(tpl)
		if tpl.Validate() == nil {
			test.Fatal("accepted invalid behavior template")
		}
	}
}

func TestBehaviorReportFocusSurvivesEveryPhase(test *testing.T) {
	behavior, err := LoadBehaviorDefault()
	if err != nil {
		test.Fatal(err)
	}
	if behavior.Version != "ocr.behavior-prompts/v9" {
		test.Fatalf("unversioned report guidance: %s", behavior.Version)
	}
	for _, phase := range []struct {
		name     string
		content  string
		required []string
	}{
		{"system", behavior.MainTask.Messages[0].Content, []string{"Report each material effect once", "practical conclusion", "not a command invocation or a second report title", "before/after table", "not a fixed length or section quota", "Preserve decisive details", "Omit empty categories", "Brevity must not narrow the investigation", "captured execution results", "recommend an option", "overall approval to merge", "If that evidence settles the contract", "omit the section when none is supported", "[[obs-000001]]"}},
		{"task", behavior.MainTask.Messages[len(behavior.MainTask.Messages)-1].Content, []string{"conclusion first", "complete visible change inventory", "decisive conditions and evidence retained", "compatibility cost", "supported recommendations"}},
		{"compression", behavior.MemoryCompressionTask.Messages[0].Content, []string{"each-effect-once", "working memory, not the final report", "Preserve evidence and unresolved questions", "[[obs-000001]]"}},
		{"final round", behavior.FinalRound.Messages[0].Content, []string{"each material effect once", "not independent risks or evidence", "precise limitations", "test source from captured execution results", "not generic approval questions", "[[obs-000001]]"}},
	} {
		test.Run(phase.name, func(test *testing.T) {
			for _, action := range []string{"pre-merge maintainer decisions", "required fix", "acceptance check", "observable pass condition", "choice"} {
				if !strings.Contains(strings.ToLower(phase.content), action) {
					test.Errorf("missing pre-merge guidance %q", action)
				}
			}
			for _, required := range phase.required {
				if !strings.Contains(phase.content, required) {
					test.Errorf("missing report guidance %q", required)
				}
			}
		})
	}
	for _, content := range []string{behavior.MainTask.Messages[0].Content, behavior.MainTask.Messages[1].Content, behavior.FinalRound.Messages[0].Content} {
		for _, required := range []string{"## for sections and ### for subsections", "standalone bold text", "at least two concrete scenarios", "evidence-supported before/after behavior", "default to a compact before/after table", "Scenario | Before | After", "single change or unverified old behavior"} {
			if !strings.Contains(content, required) {
				test.Errorf("missing presentation guidance %q", required)
			}
		}
	}
	if behavior.MaxTokens != 200000 || behavior.MaxCompletionTokens != 16384 || behavior.MaxToolRequestTimes != 300 {
		test.Fatal("concise reporting must not reduce investigation or output budgets")
	}
	catalog, err := toolsconfig.LoadDefault()
	if err != nil {
		test.Fatal(err)
	}
	definitions, err := toolsconfig.Resolve(catalog, []string{"submit_analysis"})
	if err != nil {
		test.Fatal(err)
	}
	parameters := definitions[0].Function.Parameters
	required, _ := parameters["required"].([]any)
	if len(required) != 1 || required[0] != "markdown" {
		test.Fatal("report focus must not add required submission fields")
	}
	properties := parameters["properties"].(map[string]any)
	markdown := properties["markdown"].(map[string]any)
	if description := markdown["description"].(string); !strings.Contains(description, "report each material effect once") || !strings.Contains(description, "when evidence supports a required fix, an acceptance check or a genuine choice") {
		test.Fatal("submission tool lost the concise-report guidance")
	}
}
