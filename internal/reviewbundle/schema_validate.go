package reviewbundle

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

var (
	resolvedSchemaMu sync.Mutex
	resolvedSchemas  = make(map[string]*jsonschema.Resolved)
)

func validateEmbeddedDocument(schemaBytes []byte, document []byte, label string) error {
	var instance any
	if err := json.Unmarshal(document, &instance); err != nil {
		return fmt.Errorf("invalid %s schema: %w", label, err)
	}
	resolved, err := cachedResolvedSchema(schemaBytes, label)
	if err != nil {
		return err
	}
	if err := resolved.Validate(instance); err != nil {
		return fmt.Errorf("invalid %s schema: %w", label, err)
	}
	return nil
}

func cachedResolvedSchema(schemaBytes []byte, label string) (*jsonschema.Resolved, error) {
	cacheKey := label + ":" + string(schemaBytes)
	resolvedSchemaMu.Lock()
	defer resolvedSchemaMu.Unlock()
	if resolved, ok := resolvedSchemas[cacheKey]; ok {
		return resolved, nil
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil, fmt.Errorf("load %s schema: %w", label, err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve %s schema: %w", label, err)
	}
	resolvedSchemas[cacheKey] = resolved
	return resolved, nil
}

func validateBundleDocument(document []byte) error {
	return validateEmbeddedDocument(BundleSchema(), document, "bundle")
}

func validateCommentsDocument(document []byte) error {
	return validateEmbeddedDocument(CommentsSchema(), document, "comments")
}

func validateManifestDocument(document []byte) error {
	return validateEmbeddedDocument(ManifestSchema(), document, "scan manifest")
}
