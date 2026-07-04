package viewer

import (
	"fmt"
	"regexp"
	"strings"
)

// safeSessionID matches session.OpenAgentRecorder run ID rules.
var safeSessionID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidateSessionID rejects path traversal and malformed session identifiers.
func ValidateSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}
	if strings.Contains(sessionID, "..") ||
		strings.Contains(sessionID, "/") ||
		strings.Contains(sessionID, "\\") {
		return fmt.Errorf("invalid session ID")
	}
	if !safeSessionID.MatchString(sessionID) {
		return fmt.Errorf("invalid session ID")
	}
	return nil
}
