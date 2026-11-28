package billing_setting

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/samber/lo"
)

const (
	BillingModeRatio        = "ratio"
	BillingModeTieredExpr   = "tiered_expr"
	BillingModeField        = "billing_mode"
	BillingExprField        = "billing_expr"
	PriceUnitField          = "price_unit"
	ResolutionPriceField    = "resolution_price"
	InputVideoDurationField = "input_video_duration"

	// PriceUnitPerCall 按次计费：模型价格就是每次的固定价格，不乘时长/尺寸等系数。
	PriceUnitPerCall = "per_call"
	// PriceUnitPerSecond 按秒计费：模型价格是每秒单价，会乘请求时长等系数。
	PriceUnitPerSecond = "per_second"
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr,
// billing_setting.price_unit, billing_setting.resolution_price,
// billing_setting.input_video_duration
type BillingSetting struct {
	BillingMode map[string]string `json:"billing_mode"`
	BillingExpr map[string]string `json:"billing_expr"`
	// PriceUnit 每个模型的计费单位（per_call / per_second）。
	// 未配置时沿用旧的「任务按次计费模型名」列表行为，保证历史配置不变。
	PriceUnit map[string]string `json:"price_unit"`
	// ResolutionPrice 每个模型按分辨率档位配置的绝对单价。
	// 价格单位跟随 PriceUnit：per_call 表示每次价格，per_second 表示每秒价格。
	// 档位键不写死，480p/720p/768p/1080p/4k 等任意值都可以。
	ResolutionPrice map[string]map[string]float64 `json:"resolution_price"`
	// InputVideoDuration 标记哪些按秒计费的任务模型按“输入视频时长”计费。
	// 开启后服务端会探测请求 videos 里的真实时长并覆盖客户端传入的 seconds。
	InputVideoDuration map[string]bool `json:"input_video_duration"`
}

var billingSetting = BillingSetting{
	BillingMode:        make(map[string]string),
	BillingExpr:        make(map[string]string),
	PriceUnit:          make(map[string]string),
	ResolutionPrice:    make(map[string]map[string]float64),
	InputVideoDuration: make(map[string]bool),
}

func init() {
	config.GlobalConfig.Register("billing_setting", &billingSetting)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	if mode, ok := billingSetting.BillingMode[model]; ok {
		return mode
	}
	return BillingModeRatio
}

func GetBillingExpr(model string) (string, bool) {
	expr, ok := billingSetting.BillingExpr[model]
	return expr, ok
}

func GetBillingModeCopy() map[string]string {
	return lo.Assign(billingSetting.BillingMode)
}

func GetBillingExprCopy() map[string]string {
	return lo.Assign(billingSetting.BillingExpr)
}

// GetPriceUnit 返回模型显式配置的计费单位（per_call / per_second），未配置返回空串。
func GetPriceUnit(model string) string {
	return billingSetting.PriceUnit[model]
}

func GetPriceUnitCopy() map[string]string {
	return lo.Assign(billingSetting.PriceUnit)
}

// IsPerSecondPriced 判断模型是否显式配置为按秒计费。
func IsPerSecondPriced(model string) bool {
	return billingSetting.PriceUnit[model] == PriceUnitPerSecond
}

// IsPerCallPriced 判断模型是否显式配置为按次计费。
func IsPerCallPriced(model string) bool {
	return billingSetting.PriceUnit[model] == PriceUnitPerCall
}

// IsInputVideoDurationBilling 判断模型是否开启「按输入视频时长计费」。
// 该开关只对按秒计价的模型生效；开启后由服务端探测输入视频的真实时长。
func IsInputVideoDurationBilling(model string) bool {
	return billingSetting.InputVideoDuration[model]
}

func GetInputVideoDurationCopy() map[string]bool {
	copied := make(map[string]bool, len(billingSetting.InputVideoDuration))
	for model, enabled := range billingSetting.InputVideoDuration {
		copied[model] = enabled
	}
	return copied
}

// NormalizeResolutionKey normalizes a resolution value into a stable key.
// It accepts labels ("720p", "1080P", "4k", "2160p") and dimensions in any of
// the separators providers use ("1280x720", "1920*1080", "1366×768"). The
// vertical/min side is used, which keeps portrait sizes and common 16:9 sizes
// on the expected tier.
func NormalizeResolutionKey(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return ""
	}
	v = strings.ReplaceAll(v, " ", "")

	sep := ""
	switch {
	case strings.Contains(v, "x"):
		sep = "x"
	case strings.Contains(v, "*"):
		sep = "*"
	case strings.Contains(v, "×"):
		sep = "×"
	}
	if sep != "" {
		parts := strings.SplitN(v, sep, 2)
		if len(parts) == 2 {
			w, errW := strconv.Atoi(parts[0])
			h, errH := strconv.Atoi(parts[1])
			if errW == nil && errH == nil && w > 0 && h > 0 {
				minSide := w
				if h < minSide {
					minSide = h
				}
				switch {
				case minSide >= 4000:
					return "8k"
				case minSide >= 2000:
					return "4k"
				case minSide >= 1440:
					return "1440p"
				case minSide >= 1000:
					return "1080p"
				case minSide >= 768:
					return "768p"
				case minSide >= 720:
					return "720p"
				case minSide >= 480:
					return "480p"
				default:
					return fmt.Sprintf("%dp", minSide)
				}
			}
		}
	}

	switch v {
	case "uhd", "4k", "2160p":
		return "4k"
	case "fhd", "fullhd", "1080p":
		return "1080p"
	case "hd", "720p":
		return "720p"
	case "sd", "480p":
		return "480p"
	}
	if strings.HasSuffix(v, "p") {
		return v
	}
	if n, err := strconv.Atoi(v); err == nil {
		return fmt.Sprintf("%dp", n)
	}
	return v
}

// GetResolutionPrice returns the configured absolute price for a model and
// resolution tier. Keys are matched after normalization, so "720P", "720p"
// and "1280x720" all resolve to the same entry.
func GetResolutionPrice(model string, resolution string) (float64, bool) {
	key := NormalizeResolutionKey(resolution)
	if key == "" {
		return 0, false
	}
	price, ok := GetResolutionPrices(model)[key]
	return price, ok
}

// GetResolutionPrices returns a copy of a model's resolution price table,
// keyed by normalized resolution label (e.g. "720p", "1080p", "4k"). Models
// without a configured table return nil.
//
// 多个写法可能归一化到同一档位（例如 "2160p" 与 "4k"）。为了保证结果稳定，
// 这里按 key 排序后遍历，"已经是规范写法"的键优先；重复项不会随机覆盖。
func GetResolutionPrices(model string) map[string]float64 {
	prices, ok := billingSetting.ResolutionPrice[model]
	if !ok || len(prices) == 0 {
		return nil
	}

	keys := make([]string, 0, len(prices))
	for resolution := range prices {
		keys = append(keys, resolution)
	}
	sort.Strings(keys)

	copied := make(map[string]float64, len(prices))
	for _, rawResolution := range keys {
		price := prices[rawResolution]
		if price < 0 {
			continue
		}
		key := NormalizeResolutionKey(rawResolution)
		if key == "" {
			continue
		}
		if _, exists := copied[key]; exists && !isCanonicalResolutionKey(rawResolution) {
			continue
		}
		copied[key] = price
	}
	if len(copied) == 0 {
		return nil
	}
	return copied
}

// isCanonicalResolutionKey 判断配置里的键本身是否已经是归一化形式。
func isCanonicalResolutionKey(rawResolution string) bool {
	normalizedInput := strings.ToLower(strings.TrimSpace(rawResolution))
	return normalizedInput != "" && NormalizeResolutionKey(rawResolution) == normalizedInput
}

func GetResolutionPriceCopy() map[string]map[string]float64 {
	result := make(map[string]map[string]float64, len(billingSetting.ResolutionPrice))
	for model, prices := range billingSetting.ResolutionPrice {
		copied := make(map[string]float64, len(prices))
		for resolution, price := range prices {
			copied[resolution] = price
		}
		result[model] = copied
	}
	return result
}

func GetPricingSyncData(base map[string]any) map[string]any {
	extra := make(map[string]any, 5)
	if modes := GetBillingModeCopy(); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := GetBillingExprCopy(); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	if units := GetPriceUnitCopy(); len(units) > 0 {
		extra[PriceUnitField] = units
	}
	if prices := GetResolutionPriceCopy(); len(prices) > 0 {
		extra[ResolutionPriceField] = prices
	}
	if flags := GetInputVideoDurationCopy(); len(flags) > 0 {
		extra[InputVideoDurationField] = flags
	}
	return lo.Assign(base, extra)
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

func smokeTestExpr(exprStr string) error {
	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}
	requests := []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}

	for _, v := range vectors {
		for _, request := range requests {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result %f < 0", v.P, v.C, result)
			}
		}
	}
	return nil
}
