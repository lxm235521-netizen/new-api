package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// WorkbenchModelDto 是工作台模型目录的一项：既包含管理员配置的参数能力
// （时长/分辨率/比例/参考素材/提示词优化），也包含按当前用户解析出的单价。
type WorkbenchModelDto struct {
	Model   string `json:"model"`
	Group   string `json:"group"`
	Billing string `json:"billing"`
	// UnitQuota 是按秒计费的「每秒额度」或按次计费的「每次额度」，
	// 与 relay 侧预扣费使用同一套公式，可直接用于展示预计消耗。
	UnitQuota  int     `json:"unit_quota"`
	UsePrice   bool    `json:"use_price"`
	ModelPrice float64 `json:"model_price"`
	ModelRatio float64 `json:"model_ratio"`
	// ResolutionQuota 是按分辨率档位配置的单位额度（键为归一化档位，如 720p/1080p），
	// 已乘分组倍率。为空表示该模型没有按分辨率定价，前端回退到 UnitQuota。
	ResolutionQuota map[string]int `json:"resolution_quota,omitempty"`

	// 以下是管理员为每个模型单独配置的参数能力
	Duration       operation_setting.WorkbenchDuration       `json:"duration"`
	Resolutions    []string                                  `json:"resolutions"`
	AspectRatios   []string                                  `json:"aspect_ratios"`
	References     operation_setting.WorkbenchReferences     `json:"references"`
	PromptOptimize operation_setting.WorkbenchPromptOptimize `json:"prompt_optimize"`
}

// resolveWorkbenchGroupRatio 复刻 relay/helper.HandleGroupRatio 的取值顺序：
// 优先用户专属分组倍率，否则用分组自身倍率。
func resolveWorkbenchGroupRatio(userGroup, usingGroup string) float64 {
	if ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, usingGroup); ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(usingGroup)
}

// resolveWorkbenchUnitQuota 计算单个模型的基准额度。
//
// 与 relay/helper.ModelPriceHelperPerCall 保持一致：
//   - 配了固定价格：modelPrice * QuotaPerUnit * groupRatio
//   - 只配了倍率：  modelRatio / 2 * QuotaPerUnit * groupRatio（预扣额）
//
// 按秒计费的模型在该基准上再乘时长，这一步由前端完成。
// ok 为 false 表示该模型既没有固定价格也没有倍率配置，无法预估。
func resolveWorkbenchUnitQuota(modelName string, groupRatio float64) (dto WorkbenchModelDto, ok bool) {
	modelPrice, usePrice := ratio_setting.GetModelPrice(modelName, false)
	if !usePrice {
		if defaultPrice, found := ratio_setting.GetDefaultModelPriceMap()[modelName]; found {
			modelPrice = defaultPrice
			usePrice = true
		}
	}

	if usePrice {
		return WorkbenchModelDto{
			UnitQuota:  int(modelPrice * common.QuotaPerUnit * groupRatio),
			UsePrice:   true,
			ModelPrice: modelPrice,
		}, true
	}

	modelRatio, hasRatio, _ := ratio_setting.GetModelRatio(modelName)
	if !hasRatio {
		return WorkbenchModelDto{}, false
	}

	return WorkbenchModelDto{
		UnitQuota:  int(modelRatio / 2 * common.QuotaPerUnit * groupRatio),
		UsePrice:   false,
		ModelPrice: -1,
		ModelRatio: modelRatio,
	}, true
}

// buildWorkbenchResolutionQuota 把模型的「按分辨率单价」换算成单位额度，
// 与 relay 侧 ApplyTaskResolutionPrice 的公式保持一致：
//
//	quota = price × QuotaPerUnit × 分组倍率
//
// 键直接使用工作台配置里的分辨率字符串（如 "768p"、"1024x1024"），
// 这样前端用当前选择的值就能直接查到，不用在 JS 里再复刻一遍归一化规则。
// 没有任何档位命中时返回 nil，前端回退到基础单价。
func buildWorkbenchResolutionQuota(modelName string, resolutions []string, groupRatio float64) map[string]int {
	if len(resolutions) == 0 {
		return nil
	}
	quotas := make(map[string]int, len(resolutions))
	for _, resolution := range resolutions {
		price, ok := billing_setting.GetResolutionPrice(modelName, resolution)
		if !ok {
			continue
		}
		quotas[resolution] = int(price * common.QuotaPerUnit * groupRatio)
	}
	if len(quotas) == 0 {
		return nil
	}
	return quotas
}

// GetWorkbenchModels 返回管理员为工作台挑选的模型目录，并附带按当前用户解析的价格。
// 目录为空时返回空列表，由前端回退到内置目录。
func GetWorkbenchModels(c *gin.Context) {
	userId := c.GetInt("id")

	userGroup := "default"
	if user, err := model.GetUserCache(userId); err == nil && user.Group != "" {
		userGroup = user.Group
	}

	groupRatio := resolveWorkbenchGroupRatio(userGroup, userGroup)

	setting := operation_setting.GetWorkbenchSetting()
	items := make([]WorkbenchModelDto, 0, len(setting.Models))

	for _, item := range setting.Models {
		resolved, ok := resolveWorkbenchUnitQuota(item.Model, groupRatio)
		if !ok {
			continue
		}
		resolved.Model = item.Model
		resolved.Group = item.Group
		resolved.Billing = item.Billing
		resolved.Duration = item.Duration
		resolved.Resolutions = item.Resolutions
		resolved.AspectRatios = item.AspectRatios
		resolved.References = item.References
		resolved.PromptOptimize = item.PromptOptimize
		resolved.ResolutionQuota = buildWorkbenchResolutionQuota(item.Model, item.Resolutions, groupRatio)
		items = append(items, resolved)
	}

	common.ApiSuccess(c, gin.H{
		"models":      items,
		"group_ratio": groupRatio,
	})
}
