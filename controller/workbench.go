package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
		items = append(items, resolved)
	}

	common.ApiSuccess(c, gin.H{
		"models":      items,
		"group_ratio": groupRatio,
	})
}
