package reviewbundle

import (
	"fmt"
	"io"
	"os"
)

// MaxProtocolDocumentBytes caps bundle, manifest, and comments payloads at load time.
const MaxProtocolDocumentBytes = 8 * 1024 * 1024

// ReadProtocolFile reads a protocol document from disk with the same byte cap as load.
func ReadProtocolFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readLimited(file)
}

func validateProtocolDocumentSize(encoded []byte) error {
	if int64(len(encoded)) > MaxProtocolDocumentBytes {
		return &ProtocolError{
			Code: "document_too_large",
			Message: fmt.Sprintf(
				"document exceeds %d byte protocol limit (%d bytes); reduce scope or bundle count",
				MaxProtocolDocumentBytes,
				len(encoded),
			),
		}
	}
	return nil
}

type protocolDocumentWriter struct {
	w        io.Writer
	n        int64
	limitErr error
}

func (writer *protocolDocumentWriter) Write(data []byte) (int, error) {
	writer.n += int64(len(data))
	if writer.n > MaxProtocolDocumentBytes {
		writer.limitErr = &ProtocolError{
			Code: "document_too_large",
			Message: fmt.Sprintf(
				"document exceeds %d byte protocol limit (%d bytes); reduce scope or bundle count",
				MaxProtocolDocumentBytes,
				writer.n,
			),
		}
		return 0, writer.limitErr
	}
	return writer.w.Write(data)
}

func (writer *protocolDocumentWriter) limitError() error {
	return writer.limitErr
}

func readLimited(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, MaxProtocolDocumentBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxProtocolDocumentBytes {
		return nil, &ProtocolError{
			Code:    "document_too_large",
			Message: fmt.Sprintf("document exceeds %d byte protocol limit", MaxProtocolDocumentBytes),
		}
	}
	return data, nil
}
