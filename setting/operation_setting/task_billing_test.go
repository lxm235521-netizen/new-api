package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
)

func withTaskPerCallBillingSettings(t *testing.T, modelNames []string, envPatches []string) {
	t.Helper()
	originalModelNames := append([]string(nil), taskPerCallBillingSetting.ModelNames...)
	originalEnvPatches := append([]string(nil), constant.TaskPricePatches...)
	taskPerCallBillingSetting.ModelNames = append([]string(nil), modelNames...)
	constant.TaskPricePatches = append([]string(nil), envPatches...)
	RebuildTaskPerCallBillingIndex()
	t.Cleanup(func() {
		taskPerCallBillingSetting.ModelNames = originalModelNames
		constant.TaskPricePatches = originalEnvPatches
		RebuildTaskPerCallBillingIndex()
	})
}

func TestIsTaskPerCallBillingModel(t *testing.T) {
	withTaskPerCallBillingSettings(t, []string{"configured-model", " trimmed-model ", ""}, []string{"env-model", " env-trimmed-model "})

	for _, modelName := range []string{"configured-model", "trimmed-model", "env-model", "env-trimmed-model"} {
		if !IsTaskPerCallBillingModel(modelName) {
			t.Fatalf("expected %s to use task per-call billing", modelName)
		}
	}

	for _, modelName := range []string{"missing-model", "configured", "env"} {
		if IsTaskPerCallBillingModel(modelName) {
			t.Fatalf("expected %s to use normal task billing", modelName)
		}
	}
}

func TestTaskPerCallBillingSettingUpdate(t *testing.T) {
	withTaskPerCallBillingSettings(t, nil, nil)

	cfg := config.GlobalConfig.Get(taskPerCallBillingConfigName)
	if cfg == nil {
		t.Fatal("task per-call billing config is not registered")
	}
	if err := config.UpdateConfigFromMap(cfg, map[string]string{"model_names": `["updated-model"," updated-trimmed-model "]`}); err != nil {
		t.Fatal(err)
	}
	RebuildTaskPerCallBillingIndex()

	if !IsTaskPerCallBillingModel("updated-model") || !IsTaskPerCallBillingModel("updated-trimmed-model") {
		t.Fatal("updated model names should be effective after rebuilding the index")
	}
	if IsTaskPerCallBillingModel("missing-model") {
		t.Fatal("unexpected model matched task per-call billing")
	}
}

// 模型价格里显式配置的计费单位优先于旧的「任务按次计费模型名」列表。
func TestIsTaskPerCallBillingModel_PriceUnitOverridesLegacyList(t *testing.T) {
	withTaskPerCallBillingSettings(t, []string{"legacy-model"}, nil)

	cfg := config.GlobalConfig.Get("billing_setting")
	bs, ok := cfg.(*billing_setting.BillingSetting)
	if !ok {
		t.Fatal("billing_setting config is not registered")
	}
	originalUnits := bs.PriceUnit
	bs.PriceUnit = map[string]string{
		"legacy-model": billing_setting.PriceUnitPerSecond,
		"new-model":    billing_setting.PriceUnitPerCall,
	}
	t.Cleanup(func() { bs.PriceUnit = originalUnits })

	if IsTaskPerCallBillingModel("legacy-model") {
		t.Fatal("per_second unit should override the legacy per-call list")
	}
	if !IsTaskPerCallBillingModel("new-model") {
		t.Fatal("per_call unit should enable per-call billing")
	}
	if IsTaskPerCallBillingModel("unconfigured-model") {
		t.Fatal("model without unit or legacy entry should not be per-call")
	}
}
