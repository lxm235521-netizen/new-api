package service

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// ContextKeyBillingResolution carries the resolution of the original task when
// a remix/continuation request does not repeat it.
const ContextKeyBillingResolution = "billing_resolution"

// ResolveTaskResolution extracts a normalized resolution tier from a task
// request. The explicit "resolution" field always wins; then
// metadata.resolution, a remix origin fallback, and finally size/metadata.size
// are used for older clients.
func ResolveTaskResolution(c *gin.Context, req relaycommon.TaskSubmitReq) (string, bool) {
	if value := strings.TrimSpace(req.Resolution); value != "" {
		if key := billing_setting.NormalizeResolutionKey(value); key != "" {
			return key, true
		}
	}
	if req.Metadata != nil {
		if value := metadataString(req.Metadata["resolution"]); value != "" {
			if key := billing_setting.NormalizeResolutionKey(value); key != "" {
				return key, true
			}
		}
	}
	if c != nil {
		if value := strings.TrimSpace(c.GetString(ContextKeyBillingResolution)); value != "" {
			if key := billing_setting.NormalizeResolutionKey(value); key != "" {
				return key, true
			}
		}
	}
	if value := strings.TrimSpace(req.Size); value != "" {
		if key := billing_setting.NormalizeResolutionKey(value); key != "" {
			return key, true
		}
	}
	if req.Metadata != nil {
		if value := metadataString(req.Metadata["size"]); value != "" {
			if key := billing_setting.NormalizeResolutionKey(value); key != "" {
				return key, true
			}
		}
	}
	return "", false
}

func metadataString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v > 0 {
			return strconv.Itoa(int(v))
		}
	case int:
		if v > 0 {
			return strconv.Itoa(v)
		}
	}
	return ""
}

func resolveRequestSeconds(req relaycommon.TaskSubmitReq) int {
	if req.Duration > 0 {
		return req.Duration
	}
	if value, err := strconv.Atoi(strings.TrimSpace(req.Seconds)); err == nil && value > 0 {
		return value
	}
	if req.Metadata != nil {
		switch value := req.Metadata["durationSeconds"].(type) {
		case float64:
			if value > 0 {
				return int(value)
			}
		case int:
			if value > 0 {
				return value
			}
		}
	}
	return 0
}

// ApplyTaskResolutionPrice overrides the base task price when the model has a
// resolution_price entry for the requested tier. The configured value is an
// absolute unit price: per request for per_call models, per second for
// per_second models. Adapter-provided resolution/size multipliers are removed
// so the configured absolute price is not multiplied twice.
func ApplyTaskResolutionPrice(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil {
		return false
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return false
	}

	resolution, ok := ResolveTaskResolution(c, req)
	if !ok {
		return false
	}
	// Keep the label for logs even when this model has no configured price.
	info.PriceData.Resolution = resolution

	price, ok := billing_setting.GetResolutionPrice(info.OriginModelName, resolution)
	if !ok && info.UpstreamModelName != "" && info.UpstreamModelName != info.OriginModelName {
		// Allow the price table to be keyed by the mapped upstream model too.
		price, ok = billing_setting.GetResolutionPrice(info.UpstreamModelName, resolution)
	}
	if !ok || price < 0 {
		return false
	}

	groupRatio := info.PriceData.GroupRatioInfo.GroupRatio
	quota := int(price * common.QuotaPerUnit * groupRatio)
	// The base model may have been marked free (e.g. model_price=0), but the
	// resolution-specific price can be non-zero. Recompute the flag.
	info.PriceData.FreeModel = false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatio == 0 || price == 0 {
			quota = 0
			info.PriceData.FreeModel = true
		}
	}

	info.PriceData.ModelPrice = price
	info.PriceData.ModelRatio = 0
	info.PriceData.UsePrice = true
	info.PriceData.Quota = quota

	// The configured price is absolute for this resolution, so drop adapter
	// resolution/size premiums to avoid charging them twice.
	for key := range info.PriceData.OtherRatios {
		lower := strings.ToLower(key)
		if lower == "resolution" || lower == "size" || strings.HasPrefix(lower, "resolution-") {
			delete(info.PriceData.OtherRatios, key)
		}
	}

	// Some adapters (Kling/Vidu/Jimeng/Hailuo, etc.) do not report seconds.
	// In per-second mode, fall back to the standard request duration fields.
	if !operation_setting.IsTaskPerCallBillingModel(info.OriginModelName) {
		if _, exists := info.PriceData.OtherRatios["seconds"]; !exists {
			if seconds := resolveRequestSeconds(req); seconds > 0 {
				info.PriceData.AddOtherRatio("seconds", float64(seconds))
			}
		}
	}
	return true
}
