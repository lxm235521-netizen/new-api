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

// BuildImageProxyURL constructs the public image content proxy URL for a task.
//
// 图片结果和视频走同一套代理（上游返回的多半是 http 地址，https 页面直接引用会被
// 浏览器当混合内容拦掉），只是路径换成图片自己的，语义更清楚。
func BuildImageProxyURL(taskID string) string {
	base := strings.TrimRight(strings.TrimSpace(ServerAddress), "/")
	if base == "" || strings.TrimSpace(taskID) == "" {
		return ""
	}
	return fmt.Sprintf("%s/v1/images/tasks/%s/content", base, taskID)
}
