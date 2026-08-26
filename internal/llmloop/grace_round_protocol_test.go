// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package llmloop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

// Cross-protocol reproduction: each supported wire format (openai chat
// completions, anthropic messages, openai responses) is driven end-to-end
// through a REAL protocol client against an httptest server, so the full path
// wire format -> protocol parsing -> shared RunPerFile loop -> grace round
// is exercised. The scripted conversation mirrors the live Ark failure:
// round 1 spends the budget on a context tool call (file_read), and the
// grace round answers with an explicit task_done(DONE).

type protocolFixture struct {
	name         string
	newClient    func(url string) llm.LLMClient
	fileReadBody string
	taskDoneBody string
}

func protocolFixtures() []protocolFixture {
	return []protocolFixture{
		{
			name: "openai_chat_completions",
			newClient: func(url string) llm.LLMClient {
				return llm.NewOpenAIClient(llm.ClientConfig{URL: url, APIKey: "test", Model: "fake"})
			},
			fileReadBody: `{
				"id":"chatcmpl_1","object":"chat.completion","created":1700000000,"model":"fake",
				"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[
					{"id":"call_1","type":"function","function":{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}}
				]},"finish_reason":"tool_calls"}],
				"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}
			}`,
			taskDoneBody: `{
				"id":"chatcmpl_2","object":"chat.completion","created":1700000000,"model":"fake",
				"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[
					{"id":"call_2","type":"function","function":{"name":"task_done","arguments":"{\"state\":\"DONE\"}"}}
				]},"finish_reason":"tool_calls"}],
				"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}
			}`,
		},
		{
			name: "anthropic_messages",
			newClient: func(url string) llm.LLMClient {
				return llm.NewAnthropicClient(llm.ClientConfig{URL: url, APIKey: "test", Model: "claude-test"})
			},
			fileReadBody: `{
				"id":"msg_1","type":"message","role":"assistant","model":"claude-test",
				"content":[{"type":"tool_use","id":"toolu_1","name":"file_read","input":{"path":"main.go"}}],
				"stop_reason":"tool_use",
				"usage":{"input_tokens":5,"output_tokens":5}
			}`,
			taskDoneBody: `{
				"id":"msg_2","type":"message","role":"assistant","model":"claude-test",
				"content":[{"type":"tool_use","id":"toolu_2","name":"task_done","input":{"state":"DONE"}}],
				"stop_reason":"tool_use",
				"usage":{"input_tokens":5,"output_tokens":5}
			}`,
		},
		{
			name: "openai_responses",
			newClient: func(url string) llm.LLMClient {
				return llm.NewOpenAIResponsesClient(llm.ClientConfig{URL: url, APIKey: "test", Model: "gpt-5.4"})
			},
			fileReadBody: `{
				"id":"resp_1","object":"response","model":"gpt-5.4","status":"completed",
				"output":[{"type":"function_call","call_id":"call_1","name":"file_read","arguments":"{\"path\":\"main.go\"}"}],
				"usage":{"input_tokens":5,"output_tokens":5,"total_tokens":10}
			}`,
			taskDoneBody: `{
				"id":"resp_2","object":"response","model":"gpt-5.4","status":"completed",
				"output":[{"type":"function_call","call_id":"call_2","name":"task_done","arguments":"{\"state\":\"DONE\"}"}],
				"usage":{"input_tokens":5,"output_tokens":5,"total_tokens":10}
			}`,
		},
	}
}

// scriptedProtocolServer serves body per request: first hit gets first,
// later hits get rest (repeating the last one).
func scriptedProtocolServer(t *testing.T, first, rest string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		body := rest
		if n == 1 {
			body = first
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// protocolRunResult captures what a scripted protocol run observed, so the
// all-protocols tests can assert on it without repeating the plumbing.
type protocolRunResult struct {
	completed bool
	stop      MainLoopStop
	err       error
	hits      int32
}

// runOverProtocol drives one protocol fixture through RunPerFile against a
// scripted server: the first hit answers a file_read (burning round 1) and
// later hits answer task_done(DONE). maxRounds decides where that task_done
// lands: 1 pushes it into the grace round, more leaves it in a normal round.
func runOverProtocol(t *testing.T, p protocolFixture, maxRounds int) protocolRunResult {
	t.Helper()
	srv, hits := scriptedProtocolServer(t, p.fileReadBody, p.taskDoneBody)
	runner := NewRunner(graceRoundTestDeps(p.newClient(srv.URL), maxRounds))
	completed, stop, err := runner.RunPerFile(
		context.Background(),
		[]llm.Message{llm.NewTextMessage("user", "review")},
		"main.go",
	)
	return protocolRunResult{completed: completed, stop: stop, err: err, hits: hits.Load()}
}

// TestRunPerFile_GraceRoundTaskDonePropagates_AllProtocols drives every
// supported wire format through the regression scenario: budget exhausted
// by a context tool call, then an explicit task_done(DONE) in the grace
// round. The tool call is parsed and executed on every protocol, and its
// completion must propagate on every protocol — the semantics live in the
// shared loop, so every response format benefits from the fix.
func TestRunPerFile_GraceRoundTaskDonePropagates_AllProtocols(t *testing.T) {
	for _, p := range protocolFixtures() {
		t.Run(p.name, func(t *testing.T) {
			got := runOverProtocol(t, p, 1)
			if got.err != nil {
				t.Fatalf("RunPerFile: %v", got.err)
			}

			// Both rounds must have happened: the context-tool round and the
			// grace round whose task_done was answered over this wire format.
			if got.hits != 2 {
				t.Fatalf("server hits = %d, want 2 (1 main round + 1 grace round)", got.hits)
			}
			if got.stop != StopNone {
				t.Fatalf("stop = %v, want StopNone on grace-round completion", got.stop)
			}
			if !got.completed {
				t.Fatalf("grace-round task_done(DONE) must propagate on %s: RunPerFile returned completed=false", p.name)
			}
		})
	}
}

// TestRunPerFile_TaskDoneNormalRound_AllProtocols is the control group:
// the same task_done(DONE), issued in a normal round with budget to spare,
// completes the run on every wire format. This proves each protocol's
// parsing path is sound — the dropped completion is a loop-level defect,
// not a per-format parsing defect.
func TestRunPerFile_TaskDoneNormalRound_AllProtocols(t *testing.T) {
	for _, p := range protocolFixtures() {
		t.Run(p.name, func(t *testing.T) {
			got := runOverProtocol(t, p, 5)
			if got.err != nil {
				t.Fatalf("RunPerFile: %v", got.err)
			}
			if got.hits != 2 {
				t.Fatalf("server hits = %d, want 2", got.hits)
			}
			if !got.completed {
				t.Fatalf("control failed on %s: task_done in a normal round must complete", p.name)
			}
		})
	}
}
