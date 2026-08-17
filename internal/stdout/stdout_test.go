// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package stdout

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestWriter_Default(t *testing.T) {
	w := Writer()
	if w != os.Stdout {
		t.Error("expected default Writer to be os.Stdout")
	}
}

func TestQuiet(t *testing.T) {
	restore := Quiet()

	w := Writer()
	if w != io.Discard {
		t.Error("expected Writer to be io.Discard after Quiet()")
	}

	restore()

	w = Writer()
	if w != os.Stdout {
		t.Error("expected Writer to be os.Stdout after restore")
	}
}

func TestSwap(t *testing.T) {
	var buf bytes.Buffer
	restore := Swap(&buf)

	if _, err := io.WriteString(Writer(), "captured"); err != nil {
		t.Fatalf("write to swapped writer failed: %v", err)
	}
	if got := buf.String(); got != "captured" {
		t.Errorf("expected swapped writer to capture %q, got %q", "captured", got)
	}

	restore()

	if w := Writer(); w != os.Stdout {
		t.Error("expected Writer to be os.Stdout after restore")
	}
}

func TestSwap_Composable(t *testing.T) {
	var buf bytes.Buffer
	outer := Swap(&buf)

	innerRestore := Quiet()
	if w := Writer(); w != io.Discard {
		t.Error("expected Writer to be io.Discard after nested Quiet()")
	}
	innerRestore()

	if w := Writer(); w != &buf {
		t.Error("expected Writer to be the swapped buffer after nested restore")
	}
	outer()

	if w := Writer(); w != os.Stdout {
		t.Error("expected Writer to be os.Stdout after outer restore")
	}
}
