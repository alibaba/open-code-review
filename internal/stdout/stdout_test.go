package stdout

import (
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
