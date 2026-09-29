// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package adapter

import (
	"encoding/json"
	"io"

	acp "github.com/coder/acp-go-sdk"
)

// The pinned SDK writes each complete JSON-RPC frame under its write mutex,
// but exposes no after-response callback. Publish discovery here only after
// the new-session response has been written successfully, under that same
// mutex. Calling SendNotification here would recursively acquire the mutex.
// This guarantees wire order, not a client-side registration acknowledgement.
type discoveryWriter struct {
	output *timedWriter
	agent  *Agent
}

func (w *discoveryWriter) Write(frame []byte) (int, error) {
	n, err := w.output.Write(frame)
	if err != nil || n != len(frame) {
		if err == nil {
			err = io.ErrShortWrite
		}
		w.output.abort()
		return n, err
	}
	var response struct {
		ID     json.RawMessage `json:"id"`
		Error  json.RawMessage `json:"error"`
		Result struct {
			SessionId acp.SessionId `json:"sessionId"`
		} `json:"result"`
	}
	if json.Unmarshal(frame, &response) != nil || len(response.ID) == 0 || len(response.Error) > 0 || response.Result.SessionId == "" {
		return n, nil
	}
	w.agent.mu.Lock()
	s := w.agent.sessions[response.Result.SessionId]
	publish := s != nil && !s.closing && !w.agent.closed && !s.commandsSent
	if publish {
		s.commandsSent = true
	}
	w.agent.mu.Unlock()
	if !publish {
		return n, nil
	}
	notification := acp.SessionNotification{SessionId: response.Result.SessionId, Update: commandUpdate()}
	payload, err := json.Marshal(struct {
		JSONRPC string                  `json:"jsonrpc"`
		Method  string                  `json:"method"`
		Params  acp.SessionNotification `json:"params"`
	}{"2.0", acp.ClientMethodSessionUpdate, notification})
	if err == nil {
		payload = append(payload, '\n')
		var written int
		written, err = w.output.Write(payload)
		if err == nil && written != len(payload) {
			err = io.ErrShortWrite
		}
	}
	if err != nil {
		w.output.abort()
	}
	return n, err
}

func commandUpdate() acp.SessionUpdate {
	return acp.SessionUpdate{AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
		SessionUpdate: "available_commands_update",
		AvailableCommands: []acp.AvailableCommand{
			{Name: "review", Description: "Review current Git changes by default. For a commit use /review --commit <ref>; for a range use /review --from main --to HEAD.", Input: &acp.AvailableCommandInput{Unstructured: &acp.UnstructuredCommandInput{Hint: "[--commit <ref> | --from <ref> --to <ref>] [--effort low|medium|high]"}}},
			{Name: "scan", Description: "Scan the session directory, including non-Git files. Select files or directories with /scan --path src or /scan --path a.go,b.go.", Input: &acp.AvailableCommandInput{Unstructured: &acp.UnstructuredCommandInput{Hint: "[--path <relative-path,...>] [--batch none|by-language|by-directory]"}}},
		},
	}}
}
