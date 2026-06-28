package teammemorysync

import (
	"crypto/sha256"
	"fmt"
)

// HashContent returns the per-entry checksum format used by team memory sync.
func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", sum)
}
