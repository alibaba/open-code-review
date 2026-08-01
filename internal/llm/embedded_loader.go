package llm

import (
	"embed"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

//go:embed bpe_data/*.tiktoken
var embedFS embed.FS

const embedPrefix = "bpe_data/"

// initEmbeddedLoader configures tiktoken to use embedded BPE data instead of fetching from network.
// Call this once during application startup, before any GetEncoding/EncodingForModel calls.
func InitEmbeddedLoader() {
	loader := &embeddedBpeLoader{}
	tiktoken.SetBpeLoader(loader)
}

// embeddedBpeLoader implements tiktoken.BpeLoader interface.
// It maps known encoding URLs to local embedded files, eliminating network dependency.
type embeddedBpeLoader struct{}

var urlToFileMap = map[string]string{
	"https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken": "cl100k_base.tiktoken",
	"https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken":  "o200k_base.tiktoken",
	"https://openaipublic.blob.core.windows.net/encodings/p50k_base.tiktoken":   "p50k_base.tiktoken",
	"https://openaipublic.blob.core.windows.net/encodings/r50k_base.tiktoken":   "r50k_base.tiktoken",
}

func (l *embeddedBpeLoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	if localName, ok := urlToFileMap[tiktokenBpeFile]; ok {
		return loadFromEmbed(localName)
	}
	return nil, fmt.Errorf("tiktoken encoding file %q is not embedded and cannot be fetched offline", tiktokenBpeFile)
}

func loadFromEmbed(filename string) (map[string]int, error) {
	data, err := embedFS.ReadFile(embedPrefix + filename)
	if err != nil {
		return nil, fmt.Errorf("embedded tiktoken file %q not found: %w", filename, err)
	}
	return parseBpeData(data)
}

// parseBpeData parses the base64-encoded BPE data format.
// Each line: <base64-token> <rank>
func parseBpeData(data []byte) (map[string]int, error) {
	bpeRanks := make(map[string]int)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid bpe data line: %q", line)
		}
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, fmt.Errorf("failed to decode token %q: %w", line, err)
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid rank in line %q: %w", line, err)
		}
		bpeRanks[string(token)] = rank
	}
	return bpeRanks, nil
}
