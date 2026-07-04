package reviewbundle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadProtocolFileEnforcesByteCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.json")
	content := strings.Repeat("a", MaxProtocolDocumentBytes+1)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadProtocolFile(path)
	if err == nil || !strings.Contains(err.Error(), "document exceeds") {
		t.Fatalf("ReadProtocolFile() error = %v, want size limit", err)
	}
}

func TestValidateProtocolDocumentSizeRejectsOversizedManifest(t *testing.T) {
	encoded := make([]byte, MaxProtocolDocumentBytes+1)
	err := validateProtocolDocumentSize(encoded)
	var protocolError *ProtocolError
	if !errors.As(err, &protocolError) || protocolError.Code != "document_too_large" {
		t.Fatalf("validateProtocolDocumentSize() error = %v, want document_too_large", err)
	}
}
