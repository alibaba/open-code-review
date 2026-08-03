package agent

import "os"

// hashlineAnchorsEnabled reports whether hashline anchor mode is on.
// Set OCR_HASHLINE_ANCHORS=1 to render the main-task diff with per-line
// "LINE#HASH:" anchors and let the model localize comments via the
// code_comment "anchor" field instead of (or in addition to) existing_code.
func hashlineAnchorsEnabled() bool {
	switch os.Getenv("OCR_HASHLINE_ANCHORS") {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}
