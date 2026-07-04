package reviewbundle

import _ "embed"

var (
	//go:embed schemas/agent-review-bundle-v1.json
	bundleSchema []byte

	//go:embed schemas/agent-review-comments-v1.json
	commentsSchema []byte

	//go:embed schemas/agent-review-manifest-v1.json
	manifestSchema []byte
)

// BundleSchema returns a defensive copy of the embedded bundle schema.
func BundleSchema() []byte {
	return append([]byte(nil), bundleSchema...)
}

// CommentsSchema returns a defensive copy of the embedded comments schema.
func CommentsSchema() []byte {
	return append([]byte(nil), commentsSchema...)
}

// ManifestSchema returns a defensive copy of the embedded scan manifest schema.
func ManifestSchema() []byte {
	return append([]byte(nil), manifestSchema...)
}
