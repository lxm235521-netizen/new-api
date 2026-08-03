package system_setting

import (
	"fmt"
	"strings"
)

// BuildVideoProxyURL constructs the public video content proxy URL for a task.
func BuildVideoProxyURL(taskID string) string {
	base := strings.TrimRight(strings.TrimSpace(ServerAddress), "/")
	if base == "" || strings.TrimSpace(taskID) == "" {
		return ""
	}
	return fmt.Sprintf("%s/v1/videos/%s/content", base, taskID)
}
