package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
)

func withWorkbenchModels(t *testing.T, models []WorkbenchModel) {
	t.Helper()
	original := append([]WorkbenchModel(nil), workbenchSetting.Models...)
	workbenchSetting.Models = append([]WorkbenchModel(nil), models...)
	t.Cleanup(func() {
		workbenchSetting.Models = original
	})
}

// 配置项必须按 workbench_setting.models 注册，否则后台保存不会生效。
func TestWorkbenchSettingRegistered(t *testing.T) {
	if cfg := config.GlobalConfig.Get(workbenchConfigName); cfg == nil {
		t.Fatalf("workbench config %q is not registered", workbenchConfigName)
	}
}

func TestWorkbenchSettingUpdateFromJSON(t *testing.T) {
	withWorkbenchModels(t, nil)

	cfg := config.GlobalConfig.Get(workbenchConfigName)
	if cfg == nil {
		t.Fatalf("workbench config %q is not registered", workbenchConfigName)
	}

	raw := `[{"model":"kling-v1","group":"video","billing":"per_second"},{"model":"gpt-image-1","group":"image","billing":"per_call"}]`
	if err := config.UpdateConfigFromMap(cfg, map[string]string{"models": raw}); err != nil {
		t.Fatal(err)
	}
	NormalizeWorkbenchSetting()

	models := GetWorkbenchSetting().Models
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Model != "kling-v1" || models[0].Group != WorkbenchGroupVideo || models[0].Billing != WorkbenchBillingPerSecond {
		t.Fatalf("unexpected first entry: %+v", models[0])
	}
	if models[1].Model != "gpt-image-1" || models[1].Group != WorkbenchGroupImage || models[1].Billing != WorkbenchBillingPerCall {
		t.Fatalf("unexpected second entry: %+v", models[1])
	}
}

// 带完整参数的配置必须原样（规范化后）保留。
func TestWorkbenchSettingFullRoundTrip(t *testing.T) {
	withWorkbenchModels(t, nil)

	cfg := config.GlobalConfig.Get(workbenchConfigName)
	raw := `[{
		"model":"sora-2",
		"group":"video",
		"billing":"per_second",
		"duration":{"mode":"range","min":1,"max":30,"step":2},
		"resolutions":["720p","1080p"],
		"aspect_ratios":["16:9","9:16"],
		"references":{"image":{"enabled":true,"max":9},"audio":{"enabled":false},"video":{"enabled":true,"max":1}},
		"prompt_optimize":{"enabled":true,"model":"h3-prompt-writing","system_prompt":"只输出中文"}
	}]`
	if err := config.UpdateConfigFromMap(cfg, map[string]string{"models": raw}); err != nil {
		t.Fatal(err)
	}
	NormalizeWorkbenchSetting()

	m := GetWorkbenchSetting().Models[0]
	if m.Duration.Mode != WorkbenchDurationRange || m.Duration.Min != 1 || m.Duration.Max != 30 || m.Duration.Step != 2 {
		t.Fatalf("duration not preserved: %+v", m.Duration)
	}
	if len(m.Resolutions) != 2 || len(m.AspectRatios) != 2 {
		t.Fatalf("resolutions/aspect ratios not preserved: %+v %+v", m.Resolutions, m.AspectRatios)
	}
	if !m.References.Image.Enabled || m.References.Image.Max != 9 {
		t.Fatalf("image reference not preserved: %+v", m.References.Image)
	}
	if !m.References.Video.Enabled || m.References.Video.Max != 1 {
		t.Fatalf("video reference not preserved: %+v", m.References.Video)
	}
	if m.References.Audio.Max != 0 {
		t.Fatalf("disabled audio reference should have max 0: %+v", m.References.Audio)
	}
	if !m.PromptOptimize.Enabled || m.PromptOptimize.Model != "h3-prompt-writing" || m.PromptOptimize.SystemPrompt != "只输出中文" {
		t.Fatalf("prompt optimize not preserved: %+v", m.PromptOptimize)
	}
}

// 老数据（只有 model/group/billing）要补出可用的默认参数。
func TestNormalizeWorkbenchSettingLegacyDefaults(t *testing.T) {
	withWorkbenchModels(t, []WorkbenchModel{
		{Model: "kling-v1", Group: WorkbenchGroupVideo, Billing: WorkbenchBillingPerSecond},
		{Model: "gpt-image-1", Group: WorkbenchGroupImage, Billing: WorkbenchBillingPerCall},
	})
	NormalizeWorkbenchSetting()

	models := GetWorkbenchSetting().Models

	video := models[0]
	if video.Duration.Mode != WorkbenchDurationList || len(video.Duration.Values) == 0 {
		t.Fatalf("video should get default duration list, got %+v", video.Duration)
	}
	if len(video.Resolutions) == 0 {
		t.Fatalf("video should get default resolutions, got %+v", video.Resolutions)
	}
	if !video.References.Image.Enabled || video.References.Image.Max != 4 {
		t.Fatalf("legacy entry should default to image reference enabled: %+v", video.References)
	}
	if video.PromptOptimize.Enabled {
		t.Fatalf("prompt optimize should default to disabled: %+v", video.PromptOptimize)
	}

	// 图片模型：不需要时长与分辨率项
	image := models[1]
	if image.Duration.Mode != "" || image.Duration.Values != nil {
		t.Fatalf("image model should have no duration control, got %+v", image.Duration)
	}
	if len(image.Resolutions) != 0 {
		t.Fatalf("image model should have no resolutions, got %+v", image.Resolutions)
	}
}

// 管理员显式清空的项不能被默认值覆盖（空 != 未配置）。
func TestNormalizeWorkbenchSettingExplicitEmptyWins(t *testing.T) {
	withWorkbenchModels(t, []WorkbenchModel{
		{
			Model:        "kling-v1",
			Group:        WorkbenchGroupVideo,
			Billing:      WorkbenchBillingPerSecond,
			Duration:     WorkbenchDuration{Mode: WorkbenchDurationList, Values: []int{}},
			Resolutions:  []string{},
			AspectRatios: []string{},
		},
	})
	NormalizeWorkbenchSetting()

	m := GetWorkbenchSetting().Models[0]
	if m.Duration.Mode != "" {
		t.Fatalf("explicit empty duration list should stay empty, got %+v", m.Duration)
	}
	if len(m.Resolutions) != 0 {
		t.Fatalf("explicit empty resolutions should stay empty, got %+v", m.Resolutions)
	}
	// 比例不能为空，否则用户无从选择，回落到一个安全值
	if len(m.AspectRatios) != 1 || m.AspectRatios[0] != "16:9" {
		t.Fatalf("empty aspect ratios should fall back to 16:9, got %+v", m.AspectRatios)
	}
}

func TestNormalizeReference(t *testing.T) {
	cases := []struct {
		in      WorkbenchReference
		wantOn  bool
		wantMax int
	}{
		{WorkbenchReference{Enabled: true, Max: 0}, true, 1},
		{WorkbenchReference{Enabled: true, Max: 9}, true, 9},
		{WorkbenchReference{Enabled: false, Max: 9}, false, 0},
		{WorkbenchReference{Enabled: true, Max: -3}, true, 1},
	}
	for _, c := range cases {
		got := normalizeReference(c.in)
		if got.Enabled != c.wantOn || got.Max != c.wantMax {
			t.Fatalf("normalizeReference(%+v) = %+v, want enabled=%v max=%d", c.in, got, c.wantOn, c.wantMax)
		}
	}
}

func TestNormalizeWorkbenchSettingDropsGarbage(t *testing.T) {
	withWorkbenchModels(t, []WorkbenchModel{
		{Model: "  kling-v1  ", Group: "", Billing: ""},
		{Model: "kling-v1", Group: "image", Billing: "per_call"},
		{Model: "   ", Group: "video", Billing: "per_second"},
		{Model: "sora-2", Group: "bogus", Billing: "bogus"},
	})
	NormalizeWorkbenchSetting()

	models := GetWorkbenchSetting().Models
	if len(models) != 2 {
		t.Fatalf("expected duplicates and blanks to be dropped, got %d entries: %+v", len(models), models)
	}
	if models[0].Model != "kling-v1" {
		t.Fatalf("expected trimmed model name, got %q", models[0].Model)
	}
	if models[0].Group != WorkbenchGroupVideo || models[0].Billing != WorkbenchBillingPerCall {
		t.Fatalf("expected invalid group/billing to fall back, got %+v", models[0])
	}
	if models[1].Model != "sora-2" || models[1].Group != WorkbenchGroupVideo || models[1].Billing != WorkbenchBillingPerCall {
		t.Fatalf("unexpected second entry: %+v", models[1])
	}
}

// 开了优化但没填模型：保留 enabled（不静默改写，避免与数据库状态不一致），
// 由配置页在保存时拦截、前端按 enabled && model 决定是否渲染按钮。
func TestNormalizePromptOptimizeKeepsEnabled(t *testing.T) {
	withWorkbenchModels(t, []WorkbenchModel{
		{
			Model:   "kling-v1",
			Group:   WorkbenchGroupVideo,
			Billing: WorkbenchBillingPerSecond,
			PromptOptimize: WorkbenchPromptOptimize{
				Enabled:      true,
				Model:        "  ",
				SystemPrompt: "  只输出中文  ",
			},
		},
	})
	NormalizeWorkbenchSetting()

	optimize := GetWorkbenchSetting().Models[0].PromptOptimize
	if !optimize.Enabled {
		t.Fatal("enabled must be preserved so UI state matches the stored value")
	}
	if optimize.Model != "" {
		t.Fatalf("model should be trimmed to empty, got %q", optimize.Model)
	}
	if optimize.SystemPrompt != "只输出中文" {
		t.Fatalf("system prompt should be trimmed, got %q", optimize.SystemPrompt)
	}
}

func TestNormalizeWorkbenchSettingEmpty(t *testing.T) {
	withWorkbenchModels(t, nil)
	NormalizeWorkbenchSetting()

	models := GetWorkbenchSetting().Models
	if models == nil {
		t.Fatal("empty setting should normalize to an empty slice, not nil")
	}
	if len(models) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(models))
	}
}
