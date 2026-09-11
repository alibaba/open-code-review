// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alibaba/open-code-review/acp/internal/contract"
)

func decodeResult(args []string, stdout []byte) (*Result, error) {
	if len(args) == 0 {
		return nil, newError(ErrorInvalidRequest, "OCR arguments are empty")
	}
	if len(bytes.TrimSpace(stdout)) == 0 {
		return nil, newError(ErrorDecode, "OCR stdout is empty")
	}
	switch args[0] {
	case "review":
		var result contract.ReviewResult
		if err := decodeOne(stdout, &result); err != nil {
			return nil, newError(ErrorDecode, "decode OCR review result: %v", err)
		}
		if !successfulReview(result) {
			return &Result{Review: &result}, newError(ErrorResult, "OCR review returned non-success status %q", result.Status)
		}
		return &Result{Review: &result}, nil
	case "scan":
		var result contract.ScanResult
		if err := decodeOne(stdout, &result); err != nil {
			return nil, newError(ErrorDecode, "decode OCR scan result: %v", err)
		}
		if !successfulScan(result) {
			return &Result{Scan: &result}, newError(ErrorResult, "OCR scan returned non-success status %q", result.Status)
		}
		return &Result{Scan: &result}, nil
	default:
		return nil, newError(ErrorInvalidRequest, "OCR arguments must start with review or scan, got %q", args[0])
	}
}

func decodeOne(data []byte, destination any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(destination); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON documents")
		}
		return err
	}
	return nil
}

func successfulReview(result contract.ReviewResult) bool {
	if result.Status != contract.StatusSuccess && result.Status != contract.StatusComplete && result.Status != contract.StatusCompletedWithWarnings {
		return false
	}
	if result.Manifest == nil || result.Manifest.TerminalState == "" {
		return true
	}
	state := result.Manifest.TerminalState
	return state == contract.StatusComplete || state == contract.StatusSuccess || state == contract.StatusCompletedWithWarnings
}

func successfulScan(result contract.ScanResult) bool {
	return result.Status == contract.StatusSuccess || result.Status == contract.StatusComplete || result.Status == contract.StatusCompletedWithWarnings
}
