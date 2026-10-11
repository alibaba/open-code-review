// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/tool"
)

// bigResult builds a tool payload comfortably over the ephemeral threshold.
func bigResult() string { return "+" + strings.Repeat("context payload ", 400) }

func toolPair(callID, payload string) []llm.Message {
	return []llm.Message{
		llm.NewToolCallMessage("", []llm.ToolCall{{
			ID:       callID,
			Type:     "function",
			Function: llm.FunctionCall{Name: "file_read", Arguments: `{"chunk_id":"c1"}`},
		}}, llm.NativeTurn{}, ""),
		llm.NewToolResultMessage(callID, payload),
	}
}

func TestLedger_FirstRequestSendsRawThenLaterOnesSendAReceipt(t *testing.T) {
	l := newContextLedger(MinContextResultThreshold)
	msgs := toolPair("call_1", bigResult())

	// The round that produces the result, and the round that consumes it: the
	// payload must still be there.
	first := l.prepare(msgs)
	if !strings.Contains(first[1].ExtractText(), "context payload") {
		t.Error("the round that consumes a read must still receive it raw")
	}

	second := l.prepare(msgs)
	if strings.Contains(second[1].ExtractText(), "context payload") {
		t.Error("after the model has answered, the raw payload must not be sent again")
	}
	receipt := second[1].ExtractText()
	if !strings.Contains(receipt, "[context receipt]") {
		t.Errorf("the elided payload must be replaced by a receipt, got %q", receipt)
	}
	if !strings.Contains(receipt, "call_1") {
		t.Error("a receipt must name what it stands for, so a re-read is possible")
	}
	if strings.Contains(receipt, "already inspected") {
		t.Error("a receipt reports transport, not a conclusion about the model's reasoning")
	}
}

func TestLedger_SmallResultsAreNeverElided(t *testing.T) {
	l := newContextLedger(MinContextResultThreshold)
	msgs := toolPair("call_1", "ok")

	for i := 0; i < 3; i++ {
		got := l.prepare(msgs)
		if !strings.Contains(got[1].ExtractText(), "ok") {
			t.Fatal("a status-sized result must survive every round: it carries no context to re-read")
		}
	}
	if s := l.stats(); s.ReceiptsIssued != 0 {
		t.Errorf("ReceiptsIssued = %d, want 0", s.ReceiptsIssued)
	}
}

func TestLedger_DoesNotMutateTheConversation(t *testing.T) {
	// The local history is what async memory compression summarizes and what a
	// session record shows. Rewriting it in place would erase both.
	l := newContextLedger(MinContextResultThreshold)
	msgs := toolPair("call_1", bigResult())
	original := msgs[1].ExtractText()

	l.prepare(msgs)
	l.prepare(msgs)

	if msgs[1].ExtractText() != original {
		t.Error("prepare must not rewrite the caller's conversation")
	}
	if len(msgs) != 2 {
		t.Errorf("prepare must not add or drop messages, got %d", len(msgs))
	}
}

func TestLedger_KeepsProviderNativeToolResultsAlone(t *testing.T) {
	// A tool result carried as provider content blocks is an SDK-validated
	// payload; reshaping it is not this code's business.
	l := newContextLedger(MinContextResultThreshold)
	native := llm.Message{
		Role:       "tool",
		ToolCallID: "call_1",
		Content:    []llm.ContentBlock{{Type: "tool_result", Text: bigResult()}},
	}
	got := l.prepare([]llm.Message{native})
	if _, isString := got[0].Content.(string); isString {
		t.Error("a content-block tool result must not be replaced by a string receipt")
	}
}

func TestLedger_CountsFirstSendsAndReceipts(t *testing.T) {
	l := newContextLedger(MinContextResultThreshold)
	msgs := toolPair("call_1", bigResult())
	payload := llm.CountTokens(bigResult())

	l.prepare(msgs)
	l.prepare(msgs)
	l.prepare(msgs)

	s := l.stats()
	if s.RawTokensSent != int64(payload) {
		t.Errorf("RawTokensSent = %d, want %d: a payload is charged once, when it first leaves", s.RawTokensSent, payload)
	}
	if s.ReceiptsIssued != 2 {
		t.Errorf("ReceiptsIssued = %d, want 2 (rounds after the first)", s.ReceiptsIssued)
	}
	if s.MaxEstimatedRequestTokens < s.LastEstimatedRequestTokens {
		t.Error("the maximum request estimate cannot be below the last one")
	}
	if s.LastEstimatedRequestTokens == 0 {
		t.Error("each request must record what it was estimated to carry")
	}
}

func TestLedger_TwoCallsAreTrackedSeparately(t *testing.T) {
	l := newContextLedger(MinContextResultThreshold)
	one := toolPair("call_1", bigResult())
	two := toolPair("call_2", bigResult())
	msgs := append(append([]llm.Message{}, one...), two...)

	both := l.prepare(msgs)
	if !strings.Contains(both[1].ExtractText(), "context payload") || !strings.Contains(both[3].ExtractText(), "context payload") {
		t.Error("a re-read of a different chunk is a first send and must be served raw")
	}
	l.prepare(msgs)
	if s := l.stats(); s.ReceiptsIssued != 2 {
		t.Errorf("ReceiptsIssued = %d, want 2", s.ReceiptsIssued)
	}
}

func TestLedger_ThresholdFloorAndDefault(t *testing.T) {
	if got := newContextLedger(1).threshold; got != MinContextResultThreshold {
		t.Errorf("a tiny threshold must floor, got %d", got)
	}
	if got := newContextLedger(0).threshold; got != DefaultContextResultThreshold {
		t.Errorf("an unset threshold must default, got %d", got)
	}
	if (*contextLedger)(nil).stats() != (ContextStats{}) {
		t.Error("a nil ledger must report zeroes rather than panic")
	}
}

func TestParseSkippedChunks(t *testing.T) {
	args := map[string]any{"skipped_chunks": []any{
		map[string]any{"chunk_id": "c1", "reason": "generated code"},
		map[string]any{"chunk_id": "  ", "reason": "no id"},
		"not an object",
		map[string]any{"chunk_id": "c2"},
	}}
	got := parseSkippedChunks(args)
	if len(got) != 2 {
		t.Fatalf("parseSkippedChunks = %+v, want the two usable entries", got)
	}
	if got[0].ID != "c1" || got[0].Reason != "generated code" {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].ID != "c2" || got[1].Reason != "" {
		t.Errorf("entry 1 = %+v: a missing cause must stay empty for the caller to name", got[1])
	}
	if parseSkippedChunks(map[string]any{}) != nil {
		t.Error("no declaration must parse to nothing")
	}
}

func TestRunner_ContextStatsFoldAcrossConversations(t *testing.T) {
	r := NewRunner(Deps{})
	a := newContextLedger(MinContextResultThreshold)
	msgs := toolPair("call_1", bigResult())
	a.prepare(msgs)
	a.prepare(msgs)
	a.fold(r)

	b := newContextLedger(MinContextResultThreshold)
	bmsgs := toolPair("call_2", bigResult())
	b.prepare(bmsgs)
	b.fold(r)

	st := r.ContextStats()
	if st.ContextReceipts != 1 {
		t.Errorf("ContextReceipts = %d, want 1", st.ContextReceipts)
	}
	if st.RawContextTokensSent != 2*int64(llm.CountTokens(bigResult())) {
		t.Errorf("RawTokensSent = %d, want both payloads charged once each", st.RawContextTokensSent)
	}
	if st.MaxEstimatedRequestTokens < st.LastEstimatedRequestTokens {
		t.Error("the run-wide maximum must be at least its last value")
	}
}

func TestRunner_RecordChunkStatsIsAdditive(t *testing.T) {
	r := NewRunner(Deps{})
	r.RecordChunkStats(ChunkRunStats{UniqueChunks: 3, FetchCount: 4, RefetchCount: 1, ResendTokens: 900})
	r.RecordChunkStats(ChunkRunStats{UniqueChunks: 2, FetchCount: 2, RefetchCount: 0, ResendTokens: 100})

	st := r.ContextStats()
	if st.UniqueContextChunks != 5 || st.ChunkFetchCount != 6 || st.ChunkRefetchCount != 1 || st.RawContextResendTokens != 1000 {
		t.Errorf("run chunk stats = %+v, want the sum of both groups", st)
	}
}

func TestRunMainTask_SendsContextOnceThenReceipts(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&bigPayloadProvider{})
	client := &fakeClient{responses: []*llm.ChatResponse{
		fileReadToolCallResponse("call_1", `{"file_path":"a.go"}`),
		fileReadToolCallResponse("call_2", `{"file_path":"b.go"}`),
		taskDoneResponse(),
	}}
	deps := newTestDeps(client)
	deps.Tools = reg
	deps.ContextResultThreshold = MinContextResultThreshold

	runner := NewRunner(deps)
	completed, _, err := runner.RunMainTask(context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")}, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("expected the task to complete")
	}
	if len(client.requests) < 3 {
		t.Fatalf("expected at least 3 requests, got %d", len(client.requests))
	}

	// Round 1 issues the read of a.go; round 2 receives it raw; round 3 must
	// not, even though it still carries the read of b.go that round 2 issued.
	second := client.requests[1]
	if !strings.Contains(text(second.Messages), "MARK:a.go") {
		t.Error("the request right after the read must carry the payload")
	}
	third := client.requests[2]
	if strings.Contains(text(third.Messages), "MARK:a.go") {
		t.Error("a later request must not re-send the raw payload the model already answered")
	}
	if !strings.Contains(text(third.Messages), "MARK:b.go") {
		t.Error("a different read, not yet answered, must still be served raw")
	}
	if !strings.Contains(text(third.Messages), "[context receipt]") {
		t.Error("a later request must carry the receipt in its place")
	}

	st := runner.ContextStats()
	if st.RawContextTokensSent <= 0 {
		t.Error("the run must report what it actually sent")
	}
	if st.ContextReceipts == 0 {
		t.Error("the run must report the receipts it issued")
	}
}

// bigPayloadProvider is a read tool whose result is sized like real review
// context, so the loop has something worth eliding. The marker names the file
// that was read, so a test can tell one payload from another.
type bigPayloadProvider struct{}

func (p *bigPayloadProvider) Tool() tool.Tool { return tool.FileRead }
func (p *bigPayloadProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	path, _ := args["file_path"].(string)
	return "+" + strings.Repeat("context payload ", 400) + "MARK:" + path, nil
}

// text concatenates the visible text of a request's messages.
func text(msgs []llm.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(m.ExtractText())
	}
	return b.String()
}
