// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/internal/session"
	"github.com/spf13/cobra"
)

func newSessionDiagnoseCmd() *cobra.Command {
	var repo, task, requestID, commentID string
	var asJSON bool
	var eventLine int
	cmd := &cobra.Command{
		Use:   "diagnose [flags] <session-id>",
		Short: "Drill into recorded task, request and tool execution",
		Long: "Inspect saved execution evidence by task and phase, then open a source record by its JSONL line number.\n" +
			"Use --request to follow explicit request links in new records; older records remain inspectable by task and event.\n" +
			"Use --comment to inspect a comment's original claim, location changes and captured code evidence.\n" +
			"The --comment and --event detail views always output JSON, even without --json.\n" +
			"Without --json, --event outputs only the original record; --comment outputs the full diagnostic report.\n" +
			"Use --json to output the full diagnostic report for any view.\n" +
			"Observations describe recorded failures and missing records; they do not judge whether review findings are correct.\n" +
			"A task key may name a group of files. Existing tool ok values are preserved as recorded.",
		Example: "  ocr session diagnose <session-id>\n" +
			"  ocr session diagnose <session-id> --task 'src/service.go'\n" +
			"  ocr session diagnose <session-id> --request <request-id>\n" +
			"  ocr session diagnose <session-id> --comment <evidence-id>\n" +
			"  ocr session diagnose <session-id> --event 12 --json",
		Args:              exactArgs(1),
		ValidArgsFunction: completeSessionIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("event") && eventLine <= 0 {
				return fmt.Errorf("--event must be a positive JSONL line number")
			}
			resolvedRepo, err := resolveWorkingDirForSession(repo)
			if err != nil {
				return err
			}
			report, err := session.Diagnose(resolvedRepo, args[0], session.DiagnoseOptions{TaskKey: task, RequestID: requestID, CommentID: commentID, EventLine: eventLine})
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			return printSessionDiagnostics(cmd.OutOrStdout(), report)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "root directory of the repository (default: current dir)")
	cmd.Flags().StringVar(&task, "task", "", "drill into an exact task key from the overview")
	cmd.Flags().StringVar(&requestID, "request", "", "drill into an explicitly linked request ID from the task events")
	cmd.Flags().StringVar(&commentID, "comment", "", "open a comment's captured origin and code evidence as JSON (may include source code; does not require --json)")
	cmd.Flags().IntVar(&eventLine, "event", 0, "open the original record at this positive JSONL line number as JSON (does not require --json)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the full diagnostic report as JSON (detail views already use JSON)")
	return cmd
}

func init() {
	sessionCmd.AddCommand(newSessionDiagnoseCmd())
}

func printSessionDiagnostics(w io.Writer, report *session.DiagnosticReport) error {
	if report.Comment != nil {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	if report.Event != nil {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report.Event)
	}
	outcome := "unknown"
	if manifest := report.Summary.RunManifest; manifest != nil {
		outcome = string(manifest.TerminalState)
	}
	if _, err := fmt.Fprintf(w, "Session: %s\nRecorded outcome: %s\nSession end recorded: %t\nFinding correctness: not assessed\n",
		report.Summary.SessionID, outcome, !report.Summary.Aborted); err != nil {
		return err
	}
	if report.RequestID != "" {
		if _, err := fmt.Fprintf(w, "Selected request: %s (task counts below apply to this request)\n", report.RequestID); err != nil {
			return err
		}
	}
	if manifest := report.Summary.RunManifest; manifest != nil {
		coverage := manifest.Coverage
		if _, err := fmt.Fprintf(w, "Coverage: selected=%d completed=%d reused=%d failed=%d waived=%d\n",
			len(coverage.Selected), len(coverage.Completed), len(coverage.Reused), len(coverage.Failed), len(coverage.Waived)); err != nil {
			return err
		}
		for _, item := range coverage.Failed {
			if _, err := fmt.Fprintf(w, "  failed item=%q classification=%q reason=%q\n", item.Path, item.Classification, item.Reason); err != nil {
				return err
			}
		}
		if manifest.RunFailure != nil {
			if _, err := fmt.Fprintf(w, "Run failure: classification=%q reason=%q\n", manifest.RunFailure.Classification, manifest.RunFailure.Reason); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(w, "\nObservations (%d):\n", len(report.Observations)); err != nil {
		return err
	}
	for _, obs := range report.Observations {
		if _, err := fmt.Fprintf(w, "  %s: line=%d task=%q phase=%q request_id=%q %q\n", obs.Code, obs.Line, obs.TaskKey, obs.TaskType, obs.RequestID, obs.Message); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "\nTasks (%d):\n", len(report.Tasks)); err != nil {
		return err
	}
	for _, task := range report.Tasks {
		if _, err := fmt.Fprintf(w, "  %q\n", task.Key); err != nil {
			return err
		}
		for _, phase := range task.Phases {
			if _, err := fmt.Fprintf(w, "    %q: requests=%d responses=%d llm_errors=%d tools=%d recorded_tool_failures=%d\n",
				phase.Type, phase.Requests, phase.Responses, phase.LLMErrors, phase.ToolCalls, phase.ToolFailures); err != nil {
				return err
			}
			for _, event := range phase.Events {
				if _, err := fmt.Fprintf(w, "      line=%d %s request=%d event_id=%q request_id=%q response_id=%q tool_call_id=%q tool=%q comment_id=%q\n", event.Line, event.Type, event.RequestNo, event.ID, event.RequestID, event.ResponseID, event.ToolCallID, event.ToolName, event.CommentID); err != nil {
					return err
				}
			}
		}
	}
	if len(report.Comments) > 0 {
		if _, err := fmt.Fprintf(w, "\nComment references (%d; checkpoint references include historical records):\n", len(report.Comments)); err != nil {
			return err
		}
		for _, cm := range report.Comments {
			if _, err := fmt.Fprintf(w, "  id=%q line=%d path=%q range=%d:%d code_reference=%q request_id=%q checkpoint_refs=%d checkpoint_state=%q decisions=%d\n",
				cm.ID, cm.Line, cm.Path, cm.StartLine, cm.EndLine, cm.EvidenceStatus, cm.RequestID, len(cm.Checkpoints), cm.CheckpointState, len(cm.Decisions)); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w, "\nUse --task <key> to list events, --request <id> to follow explicit links, --comment <id> to open comment evidence, and --event <line> to inspect an original record.")
	return err
}
