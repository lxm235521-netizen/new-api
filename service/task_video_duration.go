package service

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"

	"github.com/gin-gonic/gin"
)

const contextKeyInputVideoDuration = "input_video_duration_seconds"

// ApplyInputVideoDurationBilling enforces model-level "bill by input video
// duration" pricing. When the switch is on, the server probes the real
// duration(s) of the request's videos and overwrites any client-supplied
// seconds/duration, so callers cannot under-report the billable duration.
//
// It must be called after the task request has been parsed and the model name
// has been resolved, but before the price is estimated.
func ApplyInputVideoDurationBilling(c *gin.Context, info *relaycommon.RelayInfo, modelName string) error {
	if !IsInputVideoDurationBillingModel(modelName, info) {
		return nil
	}
	// Remix/continuation reuses the origin task's duration; its request usually
	// only carries a prompt and a video id, not videos[].
	if info != nil && info.TaskRelayInfo != nil && info.OriginTaskID != "" {
		return nil
	}
	if !isPerSecondPricedModel(modelName, info) {
		return fmt.Errorf("model %s uses input video duration billing but is not configured as per_second", modelName)
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return err
	}

	// Retries re-enter RelayTaskSubmit with the same gin context; do not probe
	// the same URLs again after a channel-level failure.
	if resolved, ok := c.Get(contextKeyInputVideoDuration); ok {
		if seconds, ok := resolved.(int); ok && seconds > 0 {
			req.Duration = seconds
			req.Seconds = ""
			c.Set("task_request", req)
			return nil
		}
	}

	videoURLs := collectTaskVideoURLs(req)
	if len(videoURLs) == 0 {
		return fmt.Errorf("model %s is billed by input video duration, but the request contains no video URL (expected videos[])", modelName)
	}

	totalSeconds := 0.0
	for _, videoURL := range videoURLs {
		duration, probeErr := probeRemoteVideoDuration(c.Request.Context(), videoURL)
		if probeErr != nil {
			return fmt.Errorf("failed to read input video duration from %s: %w", common.MaskSensitiveInfo(videoURL), probeErr)
		}
		totalSeconds += duration
	}

	if totalSeconds <= 0 {
		return fmt.Errorf("input video duration is zero or invalid")
	}
	if totalSeconds > videoDurationProbeMaxSecond {
		return fmt.Errorf("input video duration %.1fs exceeds the %ds limit", totalSeconds, videoDurationProbeMaxSecond)
	}

	// Providers bill whole seconds. Round up so a 15.08s video is never billed
	// as 15s; subtract a tiny epsilon to keep an exact 15.000s video at 15.
	billedSeconds := int(math.Ceil(totalSeconds - 1e-6))
	if billedSeconds < 1 {
		billedSeconds = 1
	}

	req.Duration = billedSeconds
	req.Seconds = ""
	c.Set("task_request", req)
	c.Set(contextKeyInputVideoDuration, billedSeconds)

	logger.LogInfo(c, fmt.Sprintf(
		"input video duration billing: model=%s videos=%d total=%.3fs billed=%ds",
		modelName, len(videoURLs), totalSeconds, billedSeconds,
	))
	return nil
}

func IsInputVideoDurationBillingModel(modelName string, info *relaycommon.RelayInfo) bool {
	if billing_setting.IsInputVideoDurationBilling(modelName) {
		return true
	}
	if info != nil && info.UpstreamModelName != "" && info.UpstreamModelName != modelName {
		return billing_setting.IsInputVideoDurationBilling(info.UpstreamModelName)
	}
	return false
}

func isPerSecondPricedModel(modelName string, info *relaycommon.RelayInfo) bool {
	if billing_setting.IsPerSecondPriced(modelName) {
		return true
	}
	if info != nil && info.UpstreamModelName != "" && info.UpstreamModelName != modelName {
		return billing_setting.IsPerSecondPriced(info.UpstreamModelName)
	}
	return false
}

// collectTaskVideoURLs extracts every input video URL from the unified task
// request. The top-level videos[] field is the primary form; the metadata
// variants keep compatibility with providers that wrap media in content[].
func collectTaskVideoURLs(req relaycommon.TaskSubmitReq) []string {
	raw := make([]string, 0, len(req.Videos)+2)
	raw = append(raw, req.Videos...)

	if req.Metadata != nil {
		for _, key := range []string{"videos", "video_urls", "video_url"} {
			raw = append(raw, stringValues(req.Metadata[key])...)
		}
		for _, item := range anySlice(req.Metadata["content"]) {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			raw = append(raw, stringValues(itemMap["video_url"])...)
			raw = append(raw, stringValues(itemMap["video_urls"])...)
		}
	}

	seen := make(map[string]struct{}, len(raw))
	result := make([]string, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func anySlice(value interface{}) []interface{} {
	switch v := value.(type) {
	case []interface{}:
		return v
	case []string:
		result := make([]interface{}, 0, len(v))
		for _, item := range v {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func stringValues(value interface{}) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, stringValues(item)...)
		}
		return result
	case map[string]interface{}:
		if urlValue, ok := v["url"].(string); ok {
			return []string{urlValue}
		}
		if pathValue, ok := v["path"].(string); ok {
			return []string{pathValue}
		}
	}
	return nil
}

// probeRemoteVideoDuration is the injectable seam used by tests.
var probeRemoteVideoDuration = func(ctx context.Context, videoURL string) (float64, error) {
	return GetRemoteVideoDuration(ctx, videoURL)
}
