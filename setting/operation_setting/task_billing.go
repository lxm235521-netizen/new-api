package operation_setting

import (
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

// IsTaskPerCallBillingModel reports whether a task model is configured for per-call billing.
func IsTaskPerCallBillingModel(modelName string) bool {
	idx := currentTaskPerCallBillingIndex.Load()
	if idx != nil {
		if _, ok := idx.modelNames[modelName]; ok {
			return true
		}
	}
	return common.StringsContains(constant.TaskPricePatches, modelName)
}
