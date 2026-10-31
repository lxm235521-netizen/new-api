package operation_setting

import (
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
)

const taskPerCallBillingConfigName = "task_per_call_billing_setting"

// TaskPerCallBillingSetting lists task models that should be billed once per request.
type TaskPerCallBillingSetting struct {
	ModelNames []string `json:"model_names"`
}

var taskPerCallBillingSetting = TaskPerCallBillingSetting{
	ModelNames: []string{},
}

type taskPerCallBillingIndex struct {
	modelNames map[string]struct{}
}

var currentTaskPerCallBillingIndex atomic.Pointer[taskPerCallBillingIndex]

func init() {
	config.GlobalConfig.Register(taskPerCallBillingConfigName, &taskPerCallBillingSetting)
	RebuildTaskPerCallBillingIndex()
}

// RebuildTaskPerCallBillingIndex refreshes the lock-free model lookup after settings change.
func RebuildTaskPerCallBillingIndex() {
	modelNames := make(map[string]struct{}, len(taskPerCallBillingSetting.ModelNames)+len(constant.TaskPricePatches))
	for _, modelName := range taskPerCallBillingSetting.ModelNames {
		if modelName = strings.TrimSpace(modelName); modelName != "" {
			modelNames[modelName] = struct{}{}
		}
	}
	for _, modelName := range constant.TaskPricePatches {
		if modelName = strings.TrimSpace(modelName); modelName != "" {
			modelNames[modelName] = struct{}{}
		}
	}
	currentTaskPerCallBillingIndex.Store(&taskPerCallBillingIndex{modelNames: modelNames})
}

// IsTaskPerCallBillingModel reports whether a task model is billed once per request.
//
// 判定顺序：
//  1. 模型在「模型价格」里显式配置了计费单位（billing_setting.price_unit）→ 以它为准；
//  2. 否则回退到旧的「任务按次计费模型名」列表 / TASK_PRICE_PATCH，保持历史行为不变。
func IsTaskPerCallBillingModel(modelName string) bool {
	switch billing_setting.GetPriceUnit(modelName) {
	case billing_setting.PriceUnitPerCall:
		return true
	case billing_setting.PriceUnitPerSecond:
		return false
	}
	idx := currentTaskPerCallBillingIndex.Load()
	if idx != nil {
		if _, ok := idx.modelNames[modelName]; ok {
			return true
		}
	}
	return common.StringsContains(constant.TaskPricePatches, modelName)
}
