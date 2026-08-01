package llm

import (
	"testing"
)

func TestParseBpeData_Valid(t *testing.T) {
	// "hello" base64 = "aGVsbG8="
	input := []byte("aGVsbG8= 42\nd29ybGQ= 7\n")
	ranks, err := parseBpeData(input)
	if err != nil {
		t.Fatalf("parseBpeData: %v", err)
	}
	if ranks["hello"] != 42 {
		t.Errorf("ranks[hello] = %d, want 42", ranks["hello"])
	}
	if ranks["world"] != 7 {
		t.Errorf("ranks[world] = %d, want 7", ranks["world"])
	}
}

func TestParseBpeData_EmptyLines(t *testing.T) {
	input := []byte("\n  \naGVsbG8= 1\n\n")
	ranks, err := parseBpeData(input)
	if err != nil {
		t.Fatalf("parseBpeData: %v", err)
	}
	if len(ranks) != 1 {
		t.Errorf("expected 1 entry, got %d", len(ranks))
	}
}

func TestParseBpeData_InvalidLine(t *testing.T) {
	input := []byte("nospacehere\n")
	_, err := parseBpeData(input)
	if err == nil {
		t.Error("expected error for line without space")
	}
}

func TestParseBpeData_InvalidBase64(t *testing.T) {
	input := []byte("!!!invalid 1\n")
	_, err := parseBpeData(input)
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestParseBpeData_InvalidRank(t *testing.T) {
	input := []byte("aGVsbG8= notanumber\n")
	_, err := parseBpeData(input)
	if err == nil {
		t.Error("expected error for non-integer rank")
	}
}

func TestLoadTiktokenBpe_KnownURL(t *testing.T) {
	loader := &embeddedBpeLoader{}
	ranks, err := loader.LoadTiktokenBpe("https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken")
	if err != nil {
		t.Fatalf("LoadTiktokenBpe: %v", err)
	}
	if len(ranks) == 0 {
		t.Error("expected non-empty ranks for cl100k_base")
	}
}

func TestLoadTiktokenBpe_UnknownURL(t *testing.T) {
	loader := &embeddedBpeLoader{}
	_, err := loader.LoadTiktokenBpe("https://example.com/unknown.tiktoken")
	if err == nil {
		t.Error("expected error for unknown URL")
	}
}

func TestInitEmbeddedLoader(t *testing.T) {
	InitEmbeddedLoader()
}
